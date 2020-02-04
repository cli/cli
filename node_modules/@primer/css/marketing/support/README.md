# Primer Marketing Support

> Support files are Sass variables, mixins, and functions that we import into different bases for use across components, objects, and utilities. Sharing these common properties across GitHub sites helps us to keep our styles more consistent.
>
> Most of the time to include these you'll only need to add `@import "./primer-marketing-support";` to the top of your bundle. If you want only a specific partial you can import them separately.

This repository is a module of the full [primer][primer] repository.

## Usage

The source files included are written in [Sass][sass] (`scss`) You can simply point your sass `include-path` at your `node_modules` directory and import it like this.

```scss
@import "primer-marketing-support/index.scss";
```

You can also import specific portions of the module by importing those partials from the `/lib/` folder. _Make sure you import any requirements along with the modules._

## Documentation

Find further documentation at [primer.style/css/support/marketing-variables](https://primer.style/css/support/marketing-variables).

## License

MIT &copy; [GitHub](https://github.com/)

[primer]: https://github.com/primer/css
[docs]: https://primer.style/css
[npm]: https://www.npmjs.com/
[install-npm]: https://docs.npmjs.com/getting-started/installing-node
[sass]: http://sass-lang.com/
