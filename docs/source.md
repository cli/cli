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

2. Build the project

   ```
   $ make
   ```

3. Move the resulting `bin/gh` executable to somewhere in your PATH

   ```sh
   $ sudo mv ./bin/gh /usr/local/bin/
   ```

4. Run `gh version` to check if it worked.
