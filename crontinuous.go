package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/robfig/cron"
	"gopkg.in/fsnotify.v1"
)

// --------------------------------------------------------------------------------------------
// ~ Constants
// --------------------------------------------------------------------------------------------

const logDelay = 1               // in seconds
const logBufferSize = 1024 * 512 // in byte => 512Kb

// --------------------------------------------------------------------------------------------
// ~ Variables
// --------------------------------------------------------------------------------------------

var (
	version         = "0.1.0"
	showVersionFlag = flag.Bool("version", false, "version info")
	executer        = flag.String("exec", os.Getenv("SHELL"), "shell / script to be called by the scheduler to execute the job")
	crontab         = flag.String("crontab", "/etc/crontab", "where to describe the jobs")
	cronScheduler   *cron.Cron
)

// --------------------------------------------------------------------------------------------
// ~ Struct
// --------------------------------------------------------------------------------------------

// Runnable implements cron.Job to Run() a command
type Runnable struct {
	ID            string
	Command       string
	Args          string
	Schedule      string
	buffer        []byte
	bufferPos     int
	isRunning     bool
	contextLogger *log.Entry
}

func createRunnable(command string, args string, schedule string) *Runnable {
	h := sha1.New()
	h.Write([]byte(command))
	hash := h.Sum(nil)
	id := hex.EncodeToString(hash)

	return &Runnable{
		ID:        id,
		Command:   command,
		Args:      args,
		Schedule:  schedule,
		buffer:    make([]byte, logBufferSize),
		bufferPos: 0,
		isRunning: false,
		contextLogger: log.WithFields(log.Fields{
			"id":       id,
			"schedule": schedule,
			"command":  command,
		}),
	}
}

func (r *Runnable) flushBufferPeriodically() {
	for r.isRunning {
		time.Sleep(logDelay * time.Second)
		go r.flush()
	}
}

func (r *Runnable) flush() {
	if r.bufferPos == 0 {
		return
	}
	trimmedLines := strings.TrimSpace(string(r.buffer[0:r.bufferPos]))
	r.bufferPos = 0
	r.contextLogger.WithField("output", trimmedLines).Info("command std output")
}

func (r *Runnable) logCreation() {
	r.contextLogger.Info("job created")
}

// --------------------------------------------------------------------------------------------
// ~ Main method
// --------------------------------------------------------------------------------------------

func init() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetLevel(log.DebugLevel)
}

func main() {
	flag.Parse()
	if *showVersionFlag {
		fmt.Printf("%v\n", version)
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go watchCrontab()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		for _ = range signalChan {
			if cronScheduler != nil {
				// Stop the scheduler (does not stop any jobs already running).
				cronScheduler.Stop()
			}
			fmt.Println("\nReceived an interrupt, stopping cron scheduler.")
			wg.Done()
			os.Exit(0)
		}
	}()

	cronScheduler = cron.New()
	initCron()
	wg.Wait()
}

// --------------------------------------------------------------------------------------------
// ~ Public methods
// --------------------------------------------------------------------------------------------

// Run a command as a cron.Job
func (r *Runnable) Run() {

	// test cmd
	_, err := exec.LookPath(r.Command)
	if err != nil {
		r.contextLogger.Error(err)
		return
	}

	// prepare execute cmd statement
	var cmd *exec.Cmd
	if *executer == "go" {
		cmdArgs := strings.Split(r.Args, " ")
		cmd = exec.Command(r.Command, cmdArgs...)
	} else {
		cmdString := r.Command + " " + r.Args
		cmd = exec.Command(os.Getenv("SHELL"), "-c", cmdString)
	}

	/*
		instead we use logrus for improved logging

		// set commands stderr and stdout to default stderr and stdout
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	*/

	// prepare cmd logging
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}

	// run cmd
	r.isRunning = true
	go r.flushBufferPeriodically()
	err = cmd.Start()
	if err != nil {
		r.contextLogger.Error(err)
	}

	// cmd logging piped stdout
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		message := []byte(scanner.Text() + "\n")
		length := len(message)
		if length > logBufferSize {
			r.contextLogger.Println("message received was too large")
			continue
		}
		if (length + r.bufferPos) > logBufferSize {
			r.flush()
		}
		copy(r.buffer[r.bufferPos:], message)
		r.bufferPos += length
	}
	if err := scanner.Err(); err != nil {
		//fmt.Fprintln(os.Stderr, "reading standard input:", err)
		r.contextLogger.Error(err)
	}

	// cmd logging piped stderr
	stderrScanner := bufio.NewScanner(stderr)
	for stderrScanner.Scan() {
		r.contextLogger.WithField("output", stderrScanner.Text()).Warn("command std error")
	}
	if err := stderrScanner.Err(); err != nil {
		//fmt.Fprintln(os.Stderr, "reading standard input:", err)
		r.contextLogger.Error(err)
	}

	err = cmd.Wait()
	if err != nil {
		r.contextLogger.Error(err)
	}

	r.isRunning = false
}

// --------------------------------------------------------------------------------------------
// ~ Private methods
// --------------------------------------------------------------------------------------------

func initCron() {
	// Stop the scheduler (does not stop any jobs already running).
	cronScheduler.Stop()

	// initialize a new cron
	cronScheduler = cron.New()

	// read crontab
	file, err := os.Open(*crontab)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parseCrontabLine(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Error("failed reading crontab", err)
		return
	}

	// start cron scheduler
	// Funcs are invoked in their own goroutine, asynchronously.
	cronScheduler.Start()
}

func parseCrontabLine(line string) {
	line = strings.TrimSpace(line)

	if len(line) <= 0 || strings.HasPrefix(line, "#") {
		return
	}

	replacer := strings.NewReplacer("  ", " ", "	", " ")
	line = replacer.Replace(line)

	// # ┌───────────── min (0 - 59)
	// # │ ┌────────────── hour (0 - 23)
	// # │ │ ┌─────────────── day of month (1 - 31)
	// # │ │ │ ┌──────────────── month (1 - 12)
	// # │ │ │ │ ┌───────────────── day of week (0 - 6) (0 to 6 are Sunday to Saturday, or use names; 7 is Sunday, the same as 0)
	// # │ │ │ │ │
	// # │ │ │ │ │
	// # * * * * *  command to execute

	var args string
	var substrings = strings.SplitN(line, " ", 7)
	if len(substrings) < 5 {
		return
	} else if len(substrings) >= 6 {
		args = strings.Join(substrings[6:7], " ")
	}

	var schedule = "0 " + strings.Join(substrings[:5], " ")
	var command = substrings[5]

	r := createRunnable(command, args, schedule)
	var err = cronScheduler.AddJob(schedule, r)
	if err != nil {
		r.contextLogger.Error("unable to parse schedule", err)
		//fmt.Printf("unable to parse schedule \"%s\" for command \"%s\" and args \"%s\" with error: \"%s\"", schedule, command, args, err)
		return
	}
	r.logCreation()
}

func watchCrontab() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.WithField("file", event.Name).Info("crontab updated")
					initCron()
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(*crontab)
	if err != nil {
		log.Fatal(err)
	}
	<-done
}
