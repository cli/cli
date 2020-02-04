# 12.0.0
The v12 release marks a major transition from small, single-purpose npm packages (`primer-core`, `primer-marketing`, `primer-utilities`, etc.) to a single package — `@primer/css` — which contains all of the SCSS source files in subdirectories. Here's what you need to do to migrate different parts of your app:

## npm
First, install the new package.

```sh
npm install --save @primer/css
```

If you use the `primer` package, all you'll need to do is:

```sh
npm uninstall --save primer
```

If you use other packages, such as `primer-utilities`, you will need to uninstall each one by name or use a command line tool like [jq](https://stedolan.github.io/jq/) to list them:

```sh
jq -r '.dependencies | keys[] | select(.|startswith("primer"))' package.json
```

And, if you're feeling saucy, uninstall them all by piping that to `xargs npm uninstall`. :sparkles:

## Sass
There are a couple of things you'll need to change in your Sass setup when migrating to v12. This section is intentionally vague because environments vary wildly between text editors, Sass implementations, and application frameworks. When in doubt, consult the relevant documentation, and feel free to [file an issue][help] if you think that we can help.

### Sass imports
Generally speaking, all of your Sass `@import` statements can be migrated with the following search-and-replace operations, **in the following order**:

* If you import `primer/index.scss` or `primer`, just replace `primer` with `@primer/css` and you're done!
* Otherwise...
    1. Replace `primer-marketing-` with `@primer/css/marketing/`, e.g.
        * `primer-marketing-buttons/index.scss` becomes `@primer/css/marketing/buttons/index.scss`
        * `primer-marketing-utilities/index.scss` becomes `@primer/css/marketing/utilities/index.scss`
    1. Replace `primer-` with `@primer/css/`, e.g.
        * `primer-markdown/index.scss` becomes `@primer/css/markdown/index.scss`
        * `primer-utilities/index.scss` becomes `@primer/css/utilities/index.scss`
    1. Delete `lib/` from every Primer CSS path, e.g.
        * `primer-forms/lib/form-control.scss` becomes `@primer/css/forms/form-control.scss`
        * `primer-navigation/lib/subnav.scss` becomes `@primer/css/navigation/subnav.scss`

If your text editor supports search and replace regular expressions, the following patterns should work:

| find | replace |
| :--- | :--- |
| `primer-marketing-(\w+)(\/lib)?` | `@primer/css/marketing/$1` |
| `primer-(\w+)(\/lib)?` | `@primer/css/$1` |
| `primer\b` | `@primer/css`

#### `primer-migrate`
You can also use the included [`primer-migrate` script](bin/primer-migrate):

```sh
npx -p @primer/css primer-migrate path/to/**/*.scss
```

### Sass include paths
If you've installed Primer CSS with npm, you very likely already have `node_modules` listed in your Sass `includePaths` option, and you won't need to change anything. :tada:

If you've installed Primer CSS with something _other than_ npm, or you don't know how it was installed, consult the documentation for your setup first, then [let us know][help] if you still can't figure it out.

## Fonts
The marketing-specific font files published in the [`fonts` directory](https://unpkg.com/primer-marketing-support@2.0.0/fonts/) of `primer-marketing-support@2.0.0` are published in the `fonts` directory of `@primer/css`. If you use these fonts, you'll need to do the following:

1. Update any scripts that copy the `.woff` font files from `node_modules/primer-marketing-support/fonts` into your application to look for them in `node_modules/@primer/css/fonts`.
1. Update any webpack (or other bundler) resolution rules that look for fonts in `primer-marketing-support/fonts` to look for them in `@primer/css/fonts`.
1. Customize the [`$marketing-font-path` variable](src/marketing/support/variables.scss#L1) to match the path from which they're served.

[help]: https://github.com/primer/css/issues/new?title=Help!&labels=v12,migration
