# GitHub Octicons

[![npm version](https://img.shields.io/npm/v/octicons.svg)](https://www.npmjs.org/package/octicons)
[![Build Status](https://travis-ci.org/primer/octicons.svg?branch=master)](https://travis-ci.org/primer/octicons)

> Octicons are a scalable set of icons handcrafted with <3 by GitHub.

## Install

**NOTE:** The compiled files are located in `build/`. This directory is located in the published npm package. Which means you can access it when you `npm install octicons`. You can also build this directory by following the [building octicons directions](#building-octicons). The files in the `lib/` directory are the raw source files and are not compiled or optimized.

#### npm

This repository is distributed with [npm][npm]. After [installing npm][install-npm], you can install `octicons` with this command.

```
$ npm install octicons --save
```

## Usage

For all the usages, we recommend using the CSS located in `build/build.css`. This is some simple CSS to normalize the icons and inherit colors.

### Node

After installing `npm install octicons` you can access the icons like this.

```js
var octicons = require("octicons")
octicons.alert
// { keywords: [ 'warning', 'triangle', 'exclamation', 'point' ],
//   path: '<path d="M8.865 1.52c-.18-.31-.51-.5-.87-.5s-.69.19-.87.5L.275 13.5c-.18.31-.18.69 0 1 .19.31.52.5.87.5h13.7c.36 0 .69-.19.86-.5.17-.31.18-.69.01-1L8.865 1.52zM8.995 13h-2v-2h2v2zm0-3h-2V6h2v4z"/>',
//   height: '16',
//   width: '16',
//   symbol: 'alert',
//   options:
//    { version: '1.1',
//      width: '16',
//      height: '16',
//      viewBox: '0 0 16 16',
//      class: 'octicon octicon-alert',
//      'aria-hidden': 'true' },
//   toSVG: [Function] }
```

There will be a key for every icon, with [`toSVG`](#octiconsalerttosvg) and other properties.

#### `octicons.alert.symbol`

Returns the string of the symbol name, same as the key for that icon.

```js
octicons.x.symbol
// "x"
```

#### `octicons.person.path`

Returns the string representation of the path of the icon.

```js
octicons.x.path
// <path d="M7.48 8l3.75 3.75-1.48 1.48L6 9.48l-3.75 3.75-1.48-1.48L4.52 8 .77 4.25l1.48-1.48L6 6.52l3.75-3.75 1.48 1.48z"></path>
```

#### `octicons.issue.options`

This is an object of all the attributes that will be added to the output tag.

```js
octicons.x.options
// { version: '1.1', width: '12', height: '16', viewBox: '0 0 12 16', class: 'octicon octicon-x', 'aria-hidden': 'true' }
```

#### `octicons.alert.width`

Returns the icon's true width, based on the svg view box width. _Note, this doesn't change if you scale it up with size options, it only is the natural width of the icon._

#### `octicons.alert.height`

Returns the icon's true height, based on the svg view box height. _Note, this doesn't change if you scale it up with size options, it only is the natural height of the icon._

#### `keywords`

Returns an array of keywords for the icon. The data comes from the [data file in lib](../data.json). Consider contributing more aliases for the icons.

```js
octicons.x.keywords
// ["remove", "close", "delete"]
```

#### `octicons.alert.toSVG()`

Returns a string of the `<svg>` tag.

```js
octicons.x.toSVG()
// <svg version="1.1" width="12" height="16" viewBox="0 0 12 16" class="octicon octicon-x" aria-hidden="true"><path d="M7.48 8l3.75 3.75-1.48 1.48L6 9.48l-3.75 3.75-1.48-1.48L4.52 8 .77 4.25l1.48-1.48L6 6.52l3.75-3.75 1.48 1.48z"/></svg>
```

The `.toSVG()` method accepts an optional `options` object. This is used to add CSS classnames, a11y options, and sizing.

##### class

Add more CSS classes to the `<svg>` tag.

```js
octicons.x.toSVG({ "class": "close" })
// <svg version="1.1" width="12" height="16" viewBox="0 0 12 16" class="octicon octicon-x close" aria-hidden="true"><path d="M7.48 8l3.75 3.75-1.48 1.48L6 9.48l-3.75 3.75-1.48-1.48L4.52 8 .77 4.25l1.48-1.48L6 6.52l3.75-3.75 1.48 1.48z"/></svg>
```

##### aria-label

Add accessibility `aria-label` to the icon.

```js
octicons.x.toSVG({ "aria-label": "Close the window" })
// <svg version="1.1" width="12" height="16" viewBox="0 0 12 16" class="octicon octicon-x" aria-label="Close the window" role="img"><path d="M7.48 8l3.75 3.75-1.48 1.48L6 9.48l-3.75 3.75-1.48-1.48L4.52 8 .77 4.25l1.48-1.48L6 6.52l3.75-3.75 1.48 1.48z"/></svg>
```

##### width & height

Size the SVG icon larger using `width` & `height` independently or together.

```js
octicons.x.toSVG({ "width": 45 })
// <svg version="1.1" width="45" height="60" viewBox="0 0 12 16" class="octicon octicon-x" aria-hidden="true"><path d="M7.48 8l3.75 3.75-1.48 1.48L6 9.48l-3.75 3.75-1.48-1.48L4.52 8 .77 4.25l1.48-1.48L6 6.52l3.75-3.75 1.48 1.48z"/></svg>
```

## License

(c) GitHub, Inc.

When using the GitHub logos, be sure to follow the [GitHub logo guidelines](https://github.com/logos).

[MIT](./LICENSE)  

[primer]: https://github.com/primer/primer
[docs]: http://primercss.io/
[npm]: https://www.npmjs.com/
[install-npm]: https://docs.npmjs.com/getting-started/installing-node
[sass]: http://sass-lang.com/
