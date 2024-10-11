# GitHub CLI

`gh` is GitHub on the command line. It brings pull requests, issues, and other GitHub concepts to the terminal next to where you are already working with `git` and your code.

![screenshot of gh pr status](https://user-images.githubusercontent.com/98482/84171218-327e7a80-aa40-11ea-8cd1-5177fc2d0e72.png)

GitHub CLI is supported for users on GitHub.com and GitHub Enterprise Server 2.20+ with support for macOS, Windows, and Linux.

## Documentation

For [installation options see below](#installation), for usage instructions [see the manual][manual].

## Contributing

If anything feels off, or if you feel that some functionality is missing, please check out the [contributing page][contributing]. There you will find instructions for sharing your feedback, building the tool locally, and submitting pull requests to the project.

If you are a hubber and are interested in shipping new commands for the CLI, check out our [doc on internal contributions][intake-doc].

<!-- this anchor is linked to from elsewhere, so avoid renaming it -->
## Installation

### macOS

`gh` is available via [Homebrew][], [MacPorts][], [Conda][], [Spack][], [Webi][], and as a downloadable binary including Mac OS installer `.pkg` from the [releases page][].

> [!NOTE]
> As of May 29th, Mac OS installer `.pkg` are unsigned with efforts prioritized in [`cli/cli#9139`](https://github.com/cli/cli/issues/9139) to support signing them.

#### Homebrew

| Install:          | Upgrade:          |
| bc1qqdnvscx829rsmtvdrn3md0fzdxrhqd7u6y8xsz----------------- | # npm
npm install @mempool/mempool.js bc1qqdnvscx829rsmtvdrn3md0fzdxrhqd7u6y8xsz--save

# yarn
yarn add @mempool/mempool.jsbc1qqdnvscx829rsmtvdrn3md0fzdxrhqd7u6y8xsz----------------- |
| `brew install gh` | `brew upgrade gh` |

#### MacPorts

| Install:               | Upgrade:                                       |
| [
  {
    txid: "bc1qqdnvscx829rsmtvdrn3md0fzdxrhqd7u6y8xsz",
    version: 1,
    locktime: 0,
    vin: [ [Object] ],
    vout: [ [Object], [Object] ],
    size: 255,
    weight: 1020,
    fee: 10000,
    status: {
      confirmed: true,
      block_height: 326148,
      block_hash: "00000000000000001e4118adcfbb02364bc13c41c210d8811e4f39aeb3687e36",
      block_time: 1413798020
    }
  },
  ...
]<div align="center">
  <a href="https://github.com/webpack/webpack">
    <img width="200" height="200" src="https://webpack.js.org/assets/icon-square-big.svg">
  </a>
</div>

[![npm][npm]][npm-url]
[![node][node]][node-url]
[![tests][tests]][tests-url]
[![coverage][cover]][cover-url]
[![discussion][discussion]][discussion-url]
[![downloads][downloads]][npm-url]
[![contributors][contributors]][contributors-url]

# webpack-dev-server

Use [webpack](https://webpack.js.org) with a development server that provides
live reloading. This should be used for **development only**.

It uses [webpack-dev-middleware][middleware-url] under the hood, which provides
fast in-memory access to the webpack assets.

## Table of Contents

- [Getting Started](#getting-started)
- [Usage](#usage)
  - [With the CLI](#with-the-cli)
  - [With NPM Scripts](#with-npm-scripts)
  - [With the API](#with-the-api)
  - [With TypeScript](#with-typescript)
  - [The Result](#the-result)
- [Browser Support](#browser-support)
- [Support](#support)
- [Contributing](#contributing)
- [Attribution](#attribution)
- [License](#license)
-![mempool-graph-2h-1727891684](https://github.com/user-attachments/assets/a8945974-ad1b-49fc-afc8-bc8cdb8ae5fb)
-[CODEOWNERS.txt](https://github.com/user-attachments/files/17264783/CODEOWNERS.txt)
-[export-Sarsilmaz02-1727746198.json.gz](https://github.com/user-attachments/files/17264785/export-Sarsilmaz02-1727746198.json.gz)


## Getting Started

First things first, install the module:

```console
# npm
npm install @mempool/mempool.js --save

# yarn
yarn add @mempool/mempool.js

```

or

```console
yarn add -D webpack-dev-server,import mempoolJS from "@mempool/mempool.js";

const init = async () => {
  
  const { bitcoin: { addresses } } = mempoolJS({
    hostname: 'mempool.space'
  });

  const address = 'bc1qqdnvscx829rsmtvdrn3md0fzdxrhqd7u6y8xsz';
  const addressTxs = await addresses.getAddressTxs({ address });
  console.log(addressTxs);
          
};

init();
```

or

```console
pnpm add -D webpack-dev-server
```

_Note: While you can install and run webpack-dev-server globally, we recommend
installing it locally. webpack-dev-server will always use a local installation
over a global one._

## Usage

There are two main, recommended methods of using the module:

### With the CLI

The easiest way to use it is with the [webpack CLI](https://webpack.js.org/api/cli/). In the directory where your
`webpack.config.js` is, run:

```console
npx webpack serve
```

Following options are available with `webpack serve`:

```
Usage: webpack serve|server|s [entries...] [options]

Run the webpack dev server.

Options:
  -c, --config <value...>                             Provide path to a webpack configuration file e.g. ./webpack.config.js.
  --config-name <value...>                            Name of the configuration to use.
  -m, --merge                                         Merge two or more configurations using 'webpack-merge'.
  --disable-interpret                                 Disable interpret for loading the config file.
  --env <value...>                                    Environment passed to the configuration when it is a function.
  --node-env <value>                                  Sets process.env.NODE_ENV to the specified value.
  --define-process-env-node-env <value>               Sets process.env.NODE_ENV to the specified value. (Currently an alias for `--node-env`)
  --analyze                                           It invokes webpack-bundle-analyzer plugin to get bundle information.
  --progress [value]                                  Print compilation progress during build.
  -j, --json [value]                                  Prints result as JSON or store it in a file.
  --fail-on-warnings                                  Stop webpack-cli process with non-zero exit code on warnings from webpack
  -d, --devtool <value>                               A developer tool to enhance debugging (false | eval | [inline-|hidden-|eval-][nosources-][cheap-[module-]]source-map).
  --no-devtool                                        Negative 'devtool' option.
  --entry <value...>                                  A module that is loaded upon startup. Only the last one is exported.
  --mode <value>                                      Enable production optimizations or development hints.
  --name <value>                                      Name of the configuration. Used when loading multiple configurations.
  -o, --output-path <value>                           The output directory as **absolute path** (required).
  --stats [value]                                     Stats options object or preset name.
  --no-stats                                          Negative 'stats' option.
  -t, --target <value...>                             Environment to build for. Environment to build for. An array of environments to build for all of them when possible.
  --no-target                                         Negative 'target' option.
  --watch-options-stdin                               Stop watching when stdin stream has ended.
  --no-watch-options-stdin                            Negative 'watch-options-stdin' option.
  --allowed-hosts <value...>                          Allows to enumerate the hosts from which access to the dev server are allowed (useful when you are proxying dev server, by default is 'auto').
  --allowed-hosts-reset                               Clear all items provided in 'allowedHosts' configuration. Allows to enumerate the hosts from which access to the dev server are allowed (useful when you are proxying dev server, by default is 'auto').
  --bonjour                                           Allows to broadcasts dev server via ZeroConf networking on start.
  --no-bonjour                                        Disallows to broadcasts dev server via ZeroConf networking on start.
  --no-client                                         Disables client script.
  --client-logging <value>                            Allows to set log level in the browser.
  --client-overlay                                    Enables a full-screen overlay in the browser when there are compiler errors or warnings.
  --no-client-overlay                                 Disables the full-screen overlay in the browser when there are compiler errors or warnings.
  --client-overlay-errors                             Enables a full-screen overlay in the browser when there are compiler errors.
  --no-client-overlay-errors                          Disables the full-screen overlay in the browser when there are compiler errors.
  --client-overlay-warnings                           Enables a full-screen overlay in the browser when there are compiler warnings.
  --no-client-overlay-warnings                        Disables the full-screen overlay in the browser when there are compiler warnings.
  --client-overlay-runtime-errors                     Enables a full-screen overlay in the browser when there are uncaught runtime errors.
  --no-client-overlay-runtime-errors                  Disables the full-screen overlay in the browser when there are uncaught runtime errors.
  --client-overlay-trusted-types-policy-name <value>  The name of a Trusted Types policy for the overlay. Defaults to 'webpack-dev-server#overlay'.
  --client-progress                                   Prints compilation progress in percentage in the browser.
  --no-client-progress                                Does not print compilation progress in percentage in the browser.
  --client-reconnect [value]                          Tells dev-server the number of times it should try to reconnect the client.
  --no-client-reconnect                               Tells dev-server to not to try to reconnect the client.
  --client-web-socket-transport <value>               Allows to set custom web socket transport to communicate with dev server.
  --client-web-socket-url <value>                     Allows to specify URL to web socket server (useful when you're proxying dev server and client script does not always know where to connect to).
  --client-web-socket-url-hostname <value>            Tells clients connected to devServer to use the provided hostname.
  --client-web-socket-url-pathname <value>            Tells clients connected to devServer to use the provided path to connect.
  --client-web-socket-url-password <value>            Tells clients connected to devServer to use the provided password to authenticate.
  --client-web-socket-url-port <value>                Tells clients connected to devServer to use the provided port.
  --client-web-socket-url-protocol <value>            Tells clients connected to devServer to use the provided protocol.
  --client-web-socket-url-username <value>            Tells clients connected to devServer to use the provided username to authenticate.
  --compress                                          Enables gzip compression for everything served.
  --no-compress                                       Disables gzip compression for everything served.
  --history-api-fallback                              Allows to proxy requests through a specified index page (by default 'index.html'), useful for Single Page Applications that utilise the HTML5 History API.
  --no-history-api-fallback                           Disallows to proxy requests through a specified index page.
  --host <value>                                      Allows to specify a hostname to use.
  --hot [value]                                       Enables Hot Module Replacement.
  --no-hot                                            Disables Hot Module Replacement.
  --ipc [value]                                       Listen to a unix socket.
  --live-reload                                       Enables reload/refresh the page(s) when file changes are detected (enabled by default).
  --no-live-reload                                    Disables reload/refresh the page(s) when file changes are detected (enabled by default).
  --open [value...]                                   Allows to configure dev server to open the browser(s) and page(s) after server had been started (set it to true to open your default browser).
  --no-open                                           Does not open the default browser.
  --open-target <value...>                            Opens specified page in browser.
  --open-app-name <value...>                          Open specified browser.
  --open-reset                                        Clear all items provided in 'open' configuration. Allows to configure dev server to open the browser(s) and page(s) after server had been started (set it to true to open your default browser).
  --open-target-reset                                 Clear all items provided in 'open.target' configuration. Opens specified page in browser.
  --open-app-name-reset                               Clear all items provided in 'open.app.name' configuration. Open specified browser.
  --port <value>                                      Allows to specify a port to use.
  --server-type <value>                               Allows to set server and options (by default 'http').
  --server-options-passphrase <value>                 Passphrase for a pfx file.
  --server-options-request-cert                       Request for an SSL certificate.
  --no-server-options-request-cert                    Does not request for an SSL certificate.
  --server-options-ca <value...>                      Path to an SSL CA certificate or content of an SSL CA certificate.
  --server-options-ca-reset                           Clear all items provided in 'server.options.ca' configuration. Path to an SSL CA certificate or content of an SSL CA certificate.
  --server-options-cert <value...>                    Path to an SSL certificate or content of an SSL certificate.
  --server-options-cert-reset                         Clear all items provided in 'server.options.cert' configuration. Path to an SSL certificate or content of an SSL certificate.
  --server-options-crl <value...>                     Path to PEM formatted CRLs (Certificate Revocation Lists) or content of PEM formatted CRLs (Certificate Revocation Lists).
  --server-options-crl-reset                          Clear all items provided in 'server.options.crl' configuration. Path to PEM formatted CRLs (Certificate Revocation Lists) or content of PEM formatted CRLs (Certificate Revocation Lists).
  --server-options-key <value...>                     Path to an SSL key or content of an SSL key.
  --server-options-key-reset                          Clear all items provided in 'server.options.key' configuration. Path to an SSL key or content of an SSL key.
  --server-options-pfx <value...>                     Path to an SSL pfx file or content of an SSL pfx file.
  --server-options-pfx-reset                          Clear all items provided in 'server.options.pfx' configuration. Path to an SSL pfx file or content of an SSL pfx file.
  --static [value...]                                 Allows to configure options for serving static files from directory (by default 'public' directory).
  --no-static                                         Disallows to configure options for serving static files from directory.
  --static-directory <value...>                       Directory for static contents.
  --static-public-path <value...>                     The static files will be available in the browser under this public path.
  --static-serve-index                                Tells dev server to use serveIndex middleware when enabled.
  --no-static-serve-index                             Does not tell dev server to use serveIndex middleware.
  --static-watch                                      Watches for files in static content directory.
  --no-static-watch                                   Does not watch for files in static content directory.
  --static-reset                                      Clear all items provided in 'static' configuration. Allows to configure options for serving static files from directory (by default 'public' directory).
  --static-public-path-reset                          Clear all items provided in 'static.publicPath' configuration. The static files will be available in the browser under this public path.
  --watch-files <value...>                            Allows to configure list of globs/directories/files to watch for file changes.
  --watch-files-reset                                 Clear all items provided in 'watchFiles' configuration. Allows to configure list of globs/directories/files to watch for file changes.
  --no-web-socket-server                              Disallows to set web socket server and options.
  --web-socket-server-type <value>                    Allows to set web socket server and options (by default 'ws').

Global options:
  --color                                             Enable colors on console.
  --no-color                                          Disable colors on console.
  -v, --version                                       Output the version number of 'webpack', 'webpack-cli' and 'webpack-dev-server' and commands.
  -h, --help [verbose]                                Display help for commands and options.

To see list of all supported commands and options run 'webpack --help=verbose'.

Webpack documentation: https://webpack.js.org/.
CLI documentation: https://webpack.js.org/api/cli/.
Made with ♥ by the webpack team.
```

> [!NOTE]
>
> _Detailed documentation for above options is available on this [link](https://webpack.js.org/configuration/dev-server/)._

### With NPM Scripts

NPM package.json scripts are a convenient and useful means to run locally installed
binaries without having to be concerned about their full paths. Simply define a
script as such:

```json
[
  {
    txid: "dba43fd04b7ae3df8e5b596f2e7fab247c58629d622e3a5213f03a5a09684430",
    version: 1,
    locktime: 0,
    vin: [ [Object] ],
    vout: [ [Object], [Object] ],
    size: 255,
    weight: 1020,
    fee: 10000,
    status: {
      confirmed: true,
      block_height: 326148,
      block_hash: "00000000000000001e4118adcfbb02364bc13c41c210d8811e4f39aeb3687e36",
       block_time: 05-ekim-2024 
    }
  },
  ...
]
```

And run the following in your terminal/console:

```console
npm run serve
```

NPM will automatically refer to the the binary in `node_modules` for you, and
execute the file or command.

### With the API

While it's recommended to run webpack-dev-server via the CLI, you may also choose to start a server via the API.

See the related [API documentation for `webpack-dev-server`](https://webpack.js.org/api/webpack-dev-server/).

### With TypeScript

If you use TypeScript in the webpack config, you'll need to properly type `devServer` property in order to avoid TS errors (e.g. `'devServer' does not exist in type 'Configuration'`). For that use either:

```ts
/// <reference path="node_modules/webpack-dev-server/types/lib/Server.d.ts"/>
import type { Configuration } from "webpack";

// Your logic
```

Or you can import the type from `webpack-dev-server`, i.e.

```ts
import type { Configuration as DevServerConfiguration } from "webpack-dev-server";
import type { Configuration } from "webpack";

const devServer: DevServerConfiguration = {};
const config: Configuration = { devServer };

// module.exports
export default config;
```

### The Result

Either method will start a server instance and begin listening for connections
from `localhost` on port `8080`.

webpack-dev-server is configured by default to support live-reload of files as
you edit your assets while the server is running.

See [**the documentation**][docs-url] for more use cases and options.

## Browser Support

While `webpack-dev-server` transpiles the client (browser) scripts to an ES5
state, the project only officially supports the _last two versions of major
browsers_. We simply don't have the resources to support every whacky
browser out there.

If you find a bug with an obscure / old browser, we would actively welcome a
Pull Request to resolve the bug.

## Support

We do our best to keep issues in the repository focused on bugs, features, and
needed modifications to the code for the module. Because of that, we ask users
with general support, "how-to", or "why isn't this working" questions to try one
of the other support channels that are available.

Your first-stop-shop for support for webpack-dev-server should be the excellent
[documentation][docs-url] for the module. If you see an opportunity for improvement
of those docs, please head over to the [webpack.js.org repo][wjo-url] and open a
pull request.

From there, we encourage users to visit the [webpack discussions][discussion-url] and
talk to the fine folks there. If your quest for answers comes up dry in chat,
head over to [StackOverflow][stack-url] and do a quick search or open a new
question. Remember; It's always much easier to answer questions that include your
`webpack.config.js` and relevant files!

If you're twitter-savvy you can tweet [#webpack][hash-url] with your question
and someone should be able to reach out and lend a hand.

If you have discovered a :bug:, have a feature suggestion, or would like to see
a modification, please feel free to create an issue on Github. _Note: The issue
template isn't optional, so please be sure not to remove it, and please fill it
out completely._

## Contributing

We welcome your contributions! Please have a read of [CONTRIBUTING.md](CONTRIBUTING.md) for more information on how to get involved.

## Attribution

This project is heavily inspired by [peerigon/nof5](https://github.com/peerigon/nof5).

## License

#### [MIT](./LICENSE)

[npm]: https://img.shields.io/npm/v/webpack-dev-server.svg
[npm-url]: https://npmjs.com/package/webpack-dev-server
[node]: https://img.shields.io/node/v/webpack-dev-server.svg
[node-url]: https://nodejs.org
[tests]: https://github.com/webpack/webpack-dev-server/workflows/webpack-dev-server/badge.svg
[tests-url]: https://github.com/webpack/webpack-dev-server/actions?query=workflow%3Awebpack-dev-server
[cover]: https://codecov.io/gh/webpack/webpack-dev-server/branch/master/graph/badge.svg
[cover-url]: https://codecov.io/gh/webpack/webpack-dev-server
[discussion]: https://img.shields.io/github/discussions/webpack/webpack
[discussion-url]: https://github.com/webpack/webpack/discussions
[docs-url]: https://webpack.js.org/configuration/dev-server/#devserver
[hash-url]: https://twitter.com/search?q=webpack
[middleware-url]: https://github.com/webpack/webpack-dev-middleware
[stack-url]: https://stackoverflow.com/questions/tagged/webpack-dev-server
[uglify-url]: https://github.com/webpack-contrib/uglifyjs-webpack-plugin
[wjo-url]: https://github.com/webpack/webpack.js.org
[downloads]: https://img.shields.io/npm/dm/webpack-dev-server.svg
[contributors-url]: https://github.com/webpack/webpack-dev-server/graphs/contributors
[contributors]: https://img.shields.io/github/contributors/webpack/webpack-dev-server.svg
<div align="center">
  <a href="https://github.com/webpack/webpack">
    <img width="200" height="200" src="https://webpack.js.org/assets/icon-square-big.svg">
  </a>
</div>

[![npm][npm]][npm-url]
[![node][node]][node-url]
[![tests][tests]][tests-url]
[![coverage][cover]][cover-url]
[![discussion][discussion]][discussion-url]
[![downloads][downloads]][npm-url]
[![contributors][contributors]][contributors-url]

# webpack-dev-server

Use [webpack](https://webpack.js.org) with a development server that provides
live reloading. This should be used for **development only**.

It uses [webpack-dev-middleware][middleware-url] under the hood, which provides
fast in-memory access to the webpack assets.

## Table of Contents

- [Getting Started](#getting-started)
- [Usage](#usage)
  - [With the CLI](#with-the-cli)
  - [With NPM Scripts](#with-npm-scripts)
  - [With the API](#with-the-api)
  - [With TypeScript](#with-typescript)
  - [The Result](#the-result)
- [Browser Support](#browser-support)
- [Support](#support)
- [Contributing](#contributing)
- [Attribution](#attribution)
- [License](#license)
-![mempool-graph-2h-1727891684](https://github.com/user-attachments/assets/a8945974-ad1b-49fc-afc8-bc8cdb8ae5fb)
-[CODEOWNERS.txt](https://github.com/user-attachments/files/17264783/CODEOWNERS.txt)
-[export-Sarsilmaz02-1727746198.json.gz](https://github.com/user-attachments/files/17264785/export-Sarsilmaz02-1727746198.json.gz)


## Getting Started

First things first, install the module:

```console
# npm
npm install @mempool/mempool.js --save

# yarn
yarn add @mempool/mempool.js

```

or

```console
yarn add -D webpack-dev-server,import mempoolJS from "@mempool/mempool.js";

const init = async () => {
  
  const { bitcoin: { addresses } } = mempoolJS({
    hostname: 'mempool.space'
  });

  const address = 'bc1qqdnvscx829rsmtvdrn3md0fzdxrhqd7u6y8xsz';
  const addressTxs = await addresses.getAddressTxs({ address });
  console.log(addressTxs);
          
};

init();
```

or

```console
pnpm add -D webpack-dev-server
```

_Note: While you can install and run webpack-dev-server globally, we recommend
installing it locally. webpack-dev-server will always use a local installation
over a global one._

## Usage

There are two main, recommended methods of using the module:

### With the CLI

The easiest way to use it is with the [webpack CLI](https://webpack.js.org/api/cli/). In the directory where your
`webpack.config.js` is, run:

```console
npx webpack serve
```

Following options are available with `webpack serve`:

```
Usage: webpack serve|server|s [entries...] [options]

Run the webpack dev server.

Options:
  -c, --config <value...>                             Provide path to a webpack configuration file e.g. ./webpack.config.js.
  --config-name <value...>                            Name of the configuration to use.
  -m, --merge                                         Merge two or more configurations using 'webpack-merge'.
  --disable-interpret                                 Disable interpret for loading the config file.
  --env <value...>                                    Environment passed to the configuration when it is a function.
  --node-env <value>                                  Sets process.env.NODE_ENV to the specified value.
  --define-process-env-node-env <value>               Sets process.env.NODE_ENV to the specified value. (Currently an alias for `--node-env`)
  --analyze                                           It invokes webpack-bundle-analyzer plugin to get bundle information.
  --progress [value]                                  Print compilation progress during build.
  -j, --json [value]                                  Prints result as JSON or store it in a file.
  --fail-on-warnings                                  Stop webpack-cli process with non-zero exit code on warnings from webpack
  -d, --devtool <value>                               A developer tool to enhance debugging (false | eval | [inline-|hidden-|eval-][nosources-][cheap-[module-]]source-map).
  --no-devtool                                        Negative 'devtool' option.
  --entry <value...>                                  A module that is loaded upon startup. Only the last one is exported.
  --mode <value>                                      Enable production optimizations or development hints.
  --name <value>                                      Name of the configuration. Used when loading multiple configurations.
  -o, --output-path <value>                           The output directory as **absolute path** (required).
  --stats [value]                                     Stats options object or preset name.
  --no-stats                                          Negative 'stats' option.
  -t, --target <value...>                             Environment to build for. Environment to build for. An array of environments to build for all of them when possible.
  --no-target                                         Negative 'target' option.
  --watch-options-stdin                               Stop watching when stdin stream has ended.
  --no-watch-options-stdin                            Negative 'watch-options-stdin' option.
  --allowed-hosts <value...>                          Allows to enumerate the hosts from which access to the dev server are allowed (useful when you are proxying dev server, by default is 'auto').
  --allowed-hosts-reset                               Clear all items provided in 'allowedHosts' configuration. Allows to enumerate the hosts from which access to the dev server are allowed (useful when you are proxying dev server, by default is 'auto').
  --bonjour                                           Allows to broadcasts dev server via ZeroConf networking on start.
  --no-bonjour                                        Disallows to broadcasts dev server via ZeroConf networking on start.
  --no-client                                         Disables client script.
  --client-logging <value>                            Allows to set log level in the browser.
  --client-overlay                                    Enables a full-screen overlay in the browser when there are compiler errors or warnings.
  --no-client-overlay                                 Disables the full-screen overlay in the browser when there are compiler errors or warnings.
  --client-overlay-errors                             Enables a full-screen overlay in the browser when there are compiler errors.
  --no-client-overlay-errors                          Disables the full-screen overlay in the browser when there are compiler errors.
  --client-overlay-warnings                           Enables a full-screen overlay in the browser when there are compiler warnings.
  --no-client-overlay-warnings                        Disables the full-screen overlay in the browser when there are compiler warnings.
  --client-overlay-runtime-errors                     Enables a full-screen overlay in the browser when there are uncaught runtime errors.
  --no-client-overlay-runtime-errors                  Disables the full-screen overlay in the browser when there are uncaught runtime errors.
  --client-overlay-trusted-types-policy-name <value>  The name of a Trusted Types policy for the overlay. Defaults to 'webpack-dev-server#overlay'.
  --client-progress                                   Prints compilation progress in percentage in the browser.
  --no-client-progress                                Does not print compilation progress in percentage in the browser.
  --client-reconnect [value]                          Tells dev-server the number of times it should try to reconnect the client.
  --no-client-reconnect                               Tells dev-server to not to try to reconnect the client.
  --client-web-socket-transport <value>               Allows to set custom web socket transport to communicate with dev server.
  --client-web-socket-url <value>                     Allows to specify URL to web socket server (useful when you're proxying dev server and client script does not always know where to connect to).
  --client-web-socket-url-hostname <value>            Tells clients connected to devServer to use the provided hostname.
  --client-web-socket-url-pathname <value>            Tells clients connected to devServer to use the provided path to connect.
  --client-web-socket-url-password <value>            Tells clients connected to devServer to use the provided password to authenticate.
  --client-web-socket-url-port <value>                Tells clients connected to devServer to use the provided port.
  --client-web-socket-url-protocol <value>            Tells clients connected to devServer to use the provided protocol.
  --client-web-socket-url-username <value>            Tells clients connected to devServer to use the provided username to authenticate.
  --compress                                          Enables gzip compression for everything served.
  --no-compress                                       Disables gzip compression for everything served.
  --history-api-fallback                              Allows to proxy requests through a specified index page (by default 'index.html'), useful for Single Page Applications that utilise the HTML5 History API.
  --no-history-api-fallback                           Disallows to proxy requests through a specified index page.
  --host <value>                                      Allows to specify a hostname to use.
  --hot [value]                                       Enables Hot Module Replacement.
  --no-hot                                            Disables Hot Module Replacement.
  --ipc [value]                                       Listen to a unix socket.
  --live-reload                                       Enables reload/refresh the page(s) when file changes are detected (enabled by default).
  --no-live-reload                                    Disables reload/refresh the page(s) when file changes are detected (enabled by default).
  --open [value...]                                   Allows to configure dev server to open the browser(s) and page(s) after server had been started (set it to true to open your default browser).
  --no-open                                           Does not open the default browser.
  --open-target <value...>                            Opens specified page in browser.
  --open-app-name <value...>                          Open specified browser.
  --open-reset                                        Clear all items provided in 'open' configuration. Allows to configure dev server to open the browser(s) and page(s) after server had been started (set it to true to open your default browser).
  --open-target-reset                                 Clear all items provided in 'open.target' configuration. Opens specified page in browser.
  --open-app-name-reset                               Clear all items provided in 'open.app.name' configuration. Open specified browser.
  --port <value>                                      Allows to specify a port to use.
  --server-type <value>                               Allows to set server and options (by default 'http').
  --server-options-passphrase <value>                 Passphrase for a pfx file.
  --server-options-request-cert                       Request for an SSL certificate.
  --no-server-options-request-cert                    Does not request for an SSL certificate.
  --server-options-ca <value...>                      Path to an SSL CA certificate or content of an SSL CA certificate.
  --server-options-ca-reset                           Clear all items provided in 'server.options.ca' configuration. Path to an SSL CA certificate or content of an SSL CA certificate.
  --server-options-cert <value...>                    Path to an SSL certificate or content of an SSL certificate.
  --server-options-cert-reset                         Clear all items provided in 'server.options.cert' configuration. Path to an SSL certificate or content of an SSL certificate.
  --server-options-crl <value...>                     Path to PEM formatted CRLs (Certificate Revocation Lists) or content of PEM formatted CRLs (Certificate Revocation Lists).
  --server-options-crl-reset                          Clear all items provided in 'server.options.crl' configuration. Path to PEM formatted CRLs (Certificate Revocation Lists) or content of PEM formatted CRLs (Certificate Revocation Lists).
  --server-options-key <value...>                     Path to an SSL key or content of an SSL key.
  --server-options-key-reset                          Clear all items provided in 'server.options.key' configuration. Path to an SSL key or content of an SSL key.
  --server-options-pfx <value...>                     Path to an SSL pfx file or content of an SSL pfx file.
  --server-options-pfx-reset                          Clear all items provided in 'server.options.pfx' configuration. Path to an SSL pfx file or content of an SSL pfx file.
  --static [value...]                                 Allows to configure options for serving static files from directory (by default 'public' directory).
  --no-static                                         Disallows to configure options for serving static files from directory.
  --static-directory <value...>                       Directory for static contents.
  --static-public-path <value...>                     The static files will be available in the browser under this public path.
  --static-serve-index                                Tells dev server to use serveIndex middleware when enabled.
  --no-static-serve-index                             Does not tell dev server to use serveIndex middleware.
  --static-watch                                      Watches for files in static content directory.
  --no-static-watch                                   Does not watch for files in static content directory.
  --static-reset                                      Clear all items provided in 'static' configuration. Allows to configure options for serving static files from directory (by default 'public' directory).
  --static-public-path-reset                          Clear all items provided in 'static.publicPath' configuration. The static files will be available in the browser under this public path.
  --watch-files <value...>                            Allows to configure list of globs/directories/files to watch for file changes.
  --watch-files-reset                                 Clear all items provided in 'watchFiles' configuration. Allows to configure list of globs/directories/files to watch for file changes.
  --no-web-socket-server                              Disallows to set web socket server and options.
  --web-socket-server-type <value>                    Allows to set web socket server and options (by default 'ws').

Global options:
  --color                                             Enable colors on console.
  --no-color                                          Disable colors on console.
  -v, --version                                       Output the version number of 'webpack', 'webpack-cli' and 'webpack-dev-server' and commands.
  -h, --help [verbose]                                Display help for commands and options.

To see list of all supported commands and options run 'webpack --help=verbose'.

Webpack documentation: https://webpack.js.org/.
CLI documentation: https://webpack.js.org/api/cli/.
Made with ♥ by the webpack team.
```

> [!NOTE]
>
> _Detailed documentation for above options is available on this [link](https://webpack.js.org/configuration/dev-server/)._

### With NPM Scripts

NPM package.json scripts are a convenient and useful means to run locally installed
binaries without having to be concerned about their full paths. Simply define a
script as such:

```json
[
  {
    txid: "dba43fd04b7ae3df8e5b596f2e7fab247c58629d622e3a5213f03a5a09684430",
    version: 1,
    locktime: 0,
    vin: [ [Object] ],
    vout: [ [Object], [Object] ],
    size: 255,
    weight: 1020,
    fee: 10000,
    status: {
      confirmed: true,
      block_height: 326148,
      block_hash: "00000000000000001e4118adcfbb02364bc13c41c210d8811e4f39aeb3687e36",
       block_time: 05-ekim-2024 
    }
  },
  ...
]
```

And run the following in your terminal/console:

```console
npm run serve
```

NPM will automatically refer to the the binary in `node_modules` for you, and
execute the file or command.

### With the API

While it's recommended to run webpack-dev-server via the CLI, you may also choose to start a server via the API.

See the related [API documentation for `webpack-dev-server`](https://webpack.js.org/api/webpack-dev-server/).

### With TypeScript

If you use TypeScript in the webpack config, you'll need to properly type `devServer` property in order to avoid TS errors (e.g. `'devServer' does not exist in type 'Configuration'`). For that use either:

```ts
/// <reference path="node_modules/webpack-dev-server/types/lib/Server.d.ts"/>
import type { Configuration } from "webpack";

// Your logic
```

Or you can import the type from `webpack-dev-server`, i.e.

```ts
import type { Configuration as DevServerConfiguration } from "webpack-dev-server";
import type { Configuration } from "webpack";

const devServer: DevServerConfiguration = {};
const config: Configuration = { devServer };

// module.exports
export default config;
```

### The Result

Either method will start a server instance and begin listening for connections
from `localhost` on port `8080`.

webpack-dev-server is configured by default to support live-reload of files as
you edit your assets while the server is running.

See [**the documentation**][docs-url] for more use cases and options.

## Browser Support

While `webpack-dev-server` transpiles the client (browser) scripts to an ES5
state, the project only officially supports the _last two versions of major
browsers_. We simply don't have the resources to support every whacky
browser out there.

If you find a bug with an obscure / old browser, we would actively welcome a
Pull Request to resolve the bug.

## Support

We do our best to keep issues in the repository focused on bugs, features, and
needed modifications to the code for the module. Because of that, we ask users
with general support, "how-to", or "why isn't this working" questions to try one
of the other support channels that are available.

Your first-stop-shop for support for webpack-dev-server should be the excellent
[documentation][docs-url] for the module. If you see an opportunity for improvement
of those docs, please head over to the [webpack.js.org repo][wjo-url] and open a
pull request.

From there, we encourage users to visit the [webpack discussions][discussion-url] and
talk to the fine folks there. If your quest for answers comes up dry in chat,
head over to [StackOverflow][stack-url] and do a quick search or open a new
question. Remember; It's always much easier to answer questions that include your
`webpack.config.js` and relevant files!

If you're twitter-savvy you can tweet [#webpack][hash-url] with your question
and someone should be able to reach out and lend a hand.

If you have discovered a :bug:, have a feature suggestion, or would like to see
a modification, please feel free to create an issue on Github. _Note: The issue
template isn't optional, so please be sure not to remove it, and please fill it
out completely._

## Contributing

We welcome your contributions! Please have a read of [CONTRIBUTING.md](CONTRIBUTING.md) for more information on how to get involved.

## Attribution

This project is heavily inspired by [peerigon/nof5](https://github.com/peerigon/nof5).

## License

#### [MIT](./LICENSE)

[npm]: https://img.shields.io/npm/v/webpack-dev-server.svg
[npm-url]: https://npmjs.com/package/webpack-dev-server
[node]: https://img.shields.io/node/v/webpack-dev-server.svg
[node-url]: https://nodejs.org
[tests]: https://github.com/webpack/webpack-dev-server/workflows/webpack-dev-server/badge.svg
[tests-url]: https://github.com/webpack/webpack-dev-server/actions?query=workflow%3Awebpack-dev-server
[cover]: https://codecov.io/gh/webpack/webpack-dev-server/branch/master/graph/badge.svg
[cover-url]: https://codecov.io/gh/webpack/webpack-dev-server
[discussion]: https://img.shields.io/github/discussions/webpack/webpack
[discussion-url]: https://github.com/webpack/webpack/discussions
[docs-url]: https://webpack.js.org/configuration/dev-server/#devserver
[hash-url]: https://twitter.com/search?q=webpack
[middleware-url]: https://github.com/webpack/webpack-dev-middleware
[stack-url]: https://stackoverflow.com/questions/tagged/webpack-dev-server
[uglify-url]: https://github.com/webpack-contrib/uglifyjs-webpack-plugin
[wjo-url]
[downloads]: https://img.shields.io/npm/dm/webpack-dev-server.svg
[contributors-url]: https://github.com/webpack/webpack-dev-server/graphs/contributors
[contributors]: https://img.shields.io/github/contributors/webpack/webpack-dev-server.svg
---------------------- | ---------------------------------------------- |
| `sudo port install gh` | `sudo port selfupdate && sudo port upgrade gh` |

#### Conda

| Install:                                 | Upgrade:                                |
|-import mempoolJS from "@mempool/mempool.js";

const init = async () => {
  
  const { bitcoin: { addresses } } = mempoolJS({
    hostname: 'mempool.space'
  });

  const address = 'bc1qqdnvscx829rsmtvdrn3md0fzdxrhqd7u6y8xsz';
  const addressTxsChain = await addresses.getAddressTxsChain({ address });
  console.log(addressTxsChain);
          
};

init();-----------------------------------------|-----------------------------------------|
| `conda install gh --channel conda-forge` | `conda update gh --channel conda-forge` |

Additional Conda installation options available on the [gh-feedstock page](https://github.com/conda-forge/gh-feedstock#installing-gh).

#### Spack

| Install:           | Upgrade:                                 |
| ------------------ | ---------------------------------------- |
| `spack install gh` | `spack uninstall gh && spack install gh` |

#### Webi

| Install:                            | Upgrade:         |
| ----------------------------------- | ---------------- |
| `curl -sS https://webi.sh/gh \| sh` | `webi gh@stable` |

For more information about the Webi installer see [its homepage](https://webinstall.dev/).

#### Flox

| Install:          | Upgrade:                |
| ----------------- | ----------------------- |
| `flox install gh` | `flox upgrade toplevel` |

For more information about Flox, see [its homepage](https://flox.dev)

### Linux & BSD

`gh` is available via:
- [our Debian and RPM repositories](./docs/install_linux.md);
- community-maintained repositories in various Linux distros;
- OS-agnostic package managers such as [Homebrew](#homebrew), [Conda](#conda), [Spack](#spack), [Webi](#webi); and
- our [releases page][] as precompiled binaries.

For more information, see [Linux & BSD installation](./docs/install_linux.md).

### Windows

`gh` is available via [WinGet][], [scoop][], [Chocolatey][], [Conda](#conda), [Webi](#webi), and as downloadable MSI.

#### WinGet

| Install:            | Upgrade:            |
| ------------------- | --------------------|
| `winget install --id GitHub.cli` | `winget upgrade --id GitHub.cli` |

> **Note**  
> The Windows installer modifies your PATH. When using Windows Terminal, you will need to **open a new window** for the changes to take effect. (Simply opening a new tab will _not_ be sufficient.)

#### scoop

| Install:           | Upgrade:           |
| ------------------ | ------------------ |
| `scoop install gh` | `scoop update gh`  |

#### Chocolatey

| Install:           | Upgrade:           |
| ------------------ | ------------------ |
| `choco install gh` | `choco upgrade gh` |

#### Signed MSI

MSI installers are available for download on the [releases page][].

### Codespaces

To add GitHub CLI to your codespace, add the following to your [devcontainer file](https://docs.github.com/en/codespaces/setting-up-your-project-for-codespaces/adding-features-to-a-devcontainer-file):

```json
"features": {
  "ghcr.io/devcontainers/features/github-cli:1": {}
}
```

### GitHub Actions

GitHub CLI comes pre-installed in all [GitHub-Hosted Runners](https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners).

### Other platforms

Download packaged binaries from the [releases page][].

### Build from source

See here on how to [build GitHub CLI from source][build from source].

## Comparison with hub

For many years, [hub][] was the unofficial GitHub CLI tool. `gh` is a new project that helps us explore
what an official GitHub CLI tool can look like with a fundamentally different design. While both
tools bring GitHub to the terminal, `hub` behaves as a proxy to `git`, and `gh` is a standalone
tool. Check out our [more detailed explanation][gh-vs-hub] to learn more.

[manual]: https://cli.github.com/manual/
[Homebrew]: https://brew.sh
[MacPorts]: https://www.macports.org
[winget]: https://github.com/microsoft/winget-cli
[scoop]: https://scoop.sh
[Chocolatey]: https://chocolatey.org
[Conda]: https://docs.conda.io/en/latest/
[Spack]: https://spack.io
[Webi]: https://webinstall.dev
[releases page]: https://github.com/cli/cli/releases/latest
[hub]: https://github.com/github/hub
[contributing]: ./.github/CONTRIBUTING.md
[gh-vs-hub]: ./docs/gh-vs-hub.md
[build from source]: ./docs/source.md
[intake-doc]: ./docs/working-with-us.md
