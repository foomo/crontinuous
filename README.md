[![Travis CI](https://travis-ci.org/foomo/crontinuous.svg?branch=development)](https://travis-ci.org/foomo/crontinuous)

Crontinuous
===========

Crontinuous is a time-based job scheduler, based on [robfig's go cron library](https://github.com/robfig/cron). Define cronjobs in a [crontab](http://pubs.opengroup.org/onlinepubs/9699919799/utilities/crontab.html) file which will be processed by crontinuous, similarly to linux' cron.

Build into a lightweight docker container it gives you the power to schedule tasks with a cron container, for example to periodically curl endpoints internally in your docker network.

When it comes to defining cronjobs have a look at [Tom Ryder's post about best practices](https://sanctum.geek.nz/arabesque/cron-best-practices/).

Another interesting approach is to use Alpine's crond directly to [https://getcarina.com/docs/tutorials/schedule-tasks-cron/](schedule tasks with a cron container).

License
-------

Copyright (c) foomo under the LGPL 3.0 license.
