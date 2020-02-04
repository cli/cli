# Primer CSS Development

If you've made it this far, **thank you**! We appreciate your contribution, and hope that this document helps you along the way. If you have any questions or problems, don't hesitate to [file an issue](https://github.com/primer/css/issues/new).

## Structure
Primer CSS is published to [npm] as [@primer/css]. Each of Primer CSS's "modules" lives in a subfolder under `src/` with an `index.scss` in it. Generally speaking, the styles are divided into three primary themes:

* **Core** styles (in `core/`) are common dependencies, which include support variables, native element and typography styles, buttons, navigation, tooltips, etc.
* **Product** styles (in `product/`) are specific to github.com, and include components such as avatars, labels, markdown styles, popovers, and progress indicators.
* **Marketing** styles (in `marketing/`) are specific to GitHub marketing efforts, including international and event-focused sites as well as the more design-heavy feature pages on github.com. Marketing styles include new colors and button styles, and extend the core typography and whitespace scales.

### Paths
Here's what you need to know about how the files are structured in both git and in the published npm module:

* In git, all of the SCSS source files live in the `src/` directory.
* When published, all of the files in `src/` are "hoisted" to the package root so that you can import, say, utilities with:

    ```scss
    @import "@primer/css/utilities/index.scss";
    ```

* All bundle interdependencies within Primer CSS are defined as relative imports (e.g. with `../`), so everything should work fine as long as the `@primer/css` directory is in one of your Sass include paths (i.e. `node_modules`).


## Workflow
The typical Primer workflow looks something like this:

1. `npm install` to install the development dependencies.
1. [Start Storybook](#storybook)
1. Navigate to the module you're working on and modify the SCSS and/or markdown files.
1. Test your changes in Storybook.
1. Push your work to a new branch.
1. Request a review from one of the Primer "core" team members.

## Install
Run `npm install` to install the npm dependencies.

## Docs site
The Primer CSS docs are built with React using [Primer Components](https://primer.style/components) and automatically deployed on every push to this repo with [Now]. You can run the server locally with:

```sh
npm start
```

Then visit http://localhost:3000/css to view the site.

:rotating_light: **Warning:** Next.js has a [long-running issue](https://github.com/zeit/next.js/issues/1189) with trailing slashes in URLs. Avoid visiting `http://localhost:3000/` if possible, as this may cause your development server to fail in less-than-graceful ways.

### The pages directory
The [pages directory](./pages/) contains all of the documentation files that map to URLs on the site. Because we host the site at `primer.style/css` (and because of the way that Now's path aliasing feature works), we nest all of our documentation under the [css subdirectory](./pages/css).


### URL tests
We have a script that catches inadvertent URL changes caused by renaming or deleting Markdown docs:

```sh
npm run test-urls
```

This script includes some exceptions for URLs that have been intentionally moved or removed in the process of moving away from the [GitHub Style Guide](https://styleguide.github.com/primer/), and which you will need to modify if you rename or remove either Markdown docs or their `path` frontmatter. See [#641](https://github.com/primer/css/pull/641) for more information.

## Storybook
To borrow a [metaphor from Brad Frost](http://bradfrost.com/blog/post/the-workshop-and-the-storefront/), the [docs site](#docs-site) is Primer CSS's storefront, and [Storybook] is its workshop.

Our Storybook setup allows you to view every HTML code block in Primer CSS's Markdown docs in isolation. To get started, run the Storybook server with:

```sh
npm run start-storybook
```

This should open up the site in your browser (if not, navigate to `http://localhost:8001`).

### Code blocks
All `html` fenced code blocks in `src/**/*.md` will be rendered as stories and listed under the relevant module's name in the left-hand nav. File changes should trigger a live reload automatically (after a brief delay).


## Scripts
Our [`package.json`](package.json) houses a collection of [run-scripts] that we use to maintain, test, build, and publish Primer CSS. Run `npm run <script>` with any of the following values for `<script>`:

* `fresh` does a "fresh" npm install (like `npm install -f`)
* `dist` runs `script/dist`, which creates CSS bundles of all the `index.scss` files in `src/`.
* `check-links` runs a link checker on your local development server (`localhost:3000`, started with `npm start`).
* `lint` lints both our SCSS and JavaScript source files.
* `lint-css` lints our SCSS source files.
* `lint-js` lints the JavaScript source files.
* `now-build` and `now-start` are run on [Now] to build and start the docs site server. `now-test` runs them both in order.
* `start` runs the documentation site locally (alias: `dev`).
* `test-urls` compares a (pre-generated) list of paths from the **deprecated** [Primer Style Guide](https://styleguide.github.com/primer/) to files in `pages/css`, and lets us know if we've inadvertently deleted or renamed anything.
* `test-migrate` tests the [`primer-migrate`](MIGRATING.md#primer-migrate) command line utility.

The above list may not always be up-to-date. You can list all of the available scripts by calling `npm run` with no other arguments.


[@primer/css]: https://www.npmjs.com/package/@primer/css
[run-scripts]: https://docs.npmjs.com/cli/run-script
[storybook]: https://storybook.js.org/
[now]: https://zeit.co/now
[npm]: https://www.npmjs.com/
[npx]: https://www.npmjs.com/package/npx
