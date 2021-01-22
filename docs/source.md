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

   #### Unix-like systems
   ```sh
   # installs to '/usr/local' by default; sudo may be required
   $ make install
   
   # or, install to a different location
   $ make install prefix=/path/to/gh
   ```

   #### Windows 
   ```sh
   # build the `bin\gh.exe` binary
   > go run script/build.go
   ```
   There is no install step available on Windows.

3. Run `gh version` to check if it worked.

   #### Windows
   Run `bin\gh version` to check if it worked.
