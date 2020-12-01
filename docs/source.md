# Installation from source

0. Verify that you have Go 1.13+ installed

   ```sh
   $ go version
   ```

   If `go` is not installed, follow instructions on [the Go website](https://golang.org/doc/install).

1. Clone this repository

   ```sh
   $ git clone https://github.com/cli/cli.git gh-cli
   $ cd gh-cli
   ```

2. Build and install

   ```sh
   # installs to '/usr/local' by default; sudo may be required
   $ make install
   ```

   To install to a different location:
   ```sh
   $ make install prefix=/path/to/gh
   ```

   Make sure that the `${prefix}/bin` directory is in your PATH.

3. Run `gh version` to check if it worked.
