# Test Lint CRLs

The contents of this directory are a variety of PEM-encoded CRLs uses to test
the CRL linting functions in the parent directory.

To create a new test CRL to exercise a new lint:

1. Install the `der2text` and `text2der` tools:

   ```sh
   $ go install github.com/syncsynchalt/der2text/cmds/text2der@latest
   $ go install github.com/syncsynchalt/der2text/cmds/der2text@latest
   ```

2. Use `der2text` to create an editable version of CRL you want to start with, usually `crl_good.pem`:
  
   ```sh
   $ der2text crl_good.pem > my_new_crl.txt
   ```

3. Edit the text file. See [the der2text readme](https://github.com/syncsynchalt/der2text) for details about the file format.

4. Write the new PEM file and run the tests to see if it works! Repeat steps 3 and 4 as necessary until you get the correct result.

   ```sh
   $ text2der my_new_crl.txt >| my_new_crl.pem
   $ go test ..
   ```

5. Remove the text file and commit your new CRL.

   ```sh
   $ rm my_new_crl.txt
   $ git add .
   ```
