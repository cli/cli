# Primer Marketing CSS Buttons

> Buttons are used for actions, like in forms, while textual hyperlinks are used for destinations, or moving from one page to another.

This repository is a module of the full [primer][primer] repository.

## Documentation

Find further documentation at [primer.style/css/utilities/marketing-buttons](https://primer.style/css/utilities/marketing-buttons).

## Usage

The source files included are written in [Sass][sass] (`scss`) You can simply point your sass `include-path` at your `node_modules` directory and import it like this.

```scss
@import "primer-marketing-buttons/index.scss";
```

You can also import specific portions of the module by importing those partials from the `/lib/` folder. _Make sure you import any requirements along with the modules._

## Build

For a compiled **css** version of this module, a npm script is included that will output a css version to `build/build.css`

```
$ npm run build
```

## License

MIT &copy; [GitHub](https://github.com/)

[primer]: https://github.com/primer/css
[primer-support]: https://github.com/primer/css-support
[support]: https://github.com/primer/css-support
[docs]: https://primer.style/css
[npm]: https://www.npmjs.com/
[install-npm]: https://docs.npmjs.com/getting-started/installing-node
[sass]: http://sass-lang.com/
