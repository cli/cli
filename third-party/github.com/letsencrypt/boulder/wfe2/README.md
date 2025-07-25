WFE v2
============

The `wfe2` package is copied from the `wfe` package in order to implement the
["ACME v2"](https://letsencrypt.org/2017/06/14/acme-v2-api.html) API. This design choice
was made to facilitate a clean separation between v1 and v2 code and to support
running a separate API process on a different port alongside the v1 API process.
