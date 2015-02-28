# Warren

Warren is a program to act as part of a monitoring system on a home network. It
exports data for external programs to acquire and log to timeseries databases. 
Currently, Warren exports data in a way that is intended for scraping by
[Prometheus](http://prometheus.io/).

It's largely a personal project, which may or may not be useful to others. It's
highly likely to change as my own requirements do. Currently monitors and
exports data from:

* Linux OS-exposed data.
* [CurrentCost](http://www.currentcost.com/) serial XML output.

`example.cfg` contains an example configuration, which is in the TOML
configuration language. Comments in the file should (hopefully) explain 