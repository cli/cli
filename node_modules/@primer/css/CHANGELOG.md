# 12.7.0

### :rocket: Enhancement
- Import Dropdown component; add dark variants for dropdown and text fields [#862](https://github.com/primer/css/pull/862)
- Add `.break-word` utility and improve word-break docs [#880](https://github.com/primer/css/pull/880)
- Publish deprecation data [#883](https://github.com/primer/css/pull/883)

### :memo: Documentation
- Fix mistake in flex-justify-start explanation [#877](https://github.com/primer/css/pull/877)

### :house: Internal
- Fix CSS bundle size report when adding bundles [#879](https://github.com/primer/css/pull/879)
- Migrate to GitHub Actions v2 [#881](https://github.com/primer/css/pull/881)

### Committers
- [@dcastil](https://github.com/dcastil)
- [@shawnbot](https://github.com/shawnbot)
- [@simurai](https://github.com/simurai)
- [@vdepizzol](https://github.com/vdepizzol)

# 12.6.2

### :nail_care: Polish
- Add default background-color to SideNav https://github.com/primer/css/pull/873

### :house: Internal
- Change order in RELEASING.md https://github.com/primer/css/pull/875

### Committers
- [@shawnbot](https://github.com/shawnbot)
- [@simurai](https://github.com/simurai)

# 12.6.1

### :bug: Bug Fix
- Remove non-ascii characters (en dashes?) from SCSS comments [#870](https://github.com/primer/css/pull/870)
- Fix SideNav "jumping" in Safari [#868](https://github.com/primer/css/pull/868)

### :nail_care: Polish
- Improve Select Menu spacing [#844](https://github.com/primer/css/pull/844)

### :memo: Documentation
- Update colorable URL [#867](https://github.com/primer/css/pull/867)

### :house: Internal
- Update changelog for 12.6.0 [#866](https://github.com/primer/css/pull/866)

### Committers
- [@jonrohan](https://github.com/jonrohan)
- [@kiendang](https://github.com/kiendang)
- [@shawnbot](https://github.com/shawnbot)
- [@shreve](https://github.com/shreve)
- [@simurai](https://github.com/simurai)

# 12.6.0

### 🚀 Enhancements
- Loading toast styles [#852](https://github.com/primer/css/pull/852)
- Header component [#845](https://github.com/primer/css/pull/845)
- Import `.octicon` CSS in the core bundle [#857](https://github.com/primer/css/pull/857)
- Design update for blankslates [#861](https://github.com/primer/css/pull/861)
- Don't let State labels wrap [#863](https://github.com/primer/css/pull/863)

### 📝 Documentation
- Fix some links in linting docs [#856](https://github.com/primer/css/pull/856)
- Updates to the development docs [#855](https://github.com/primer/css/pull/855)
- Generate bundle source READMEs from a template [#859](https://github.com/primer/css/pull/859)
- Render Octicon Ruby helper in docs [#847](https://github.com/primer/css/pull/847)

### 🏠 Internal
- Add GitHub `styleguide.css` to Storybook [#849](https://github.com/primer/css/pull/849)

### Committers
- [@ashygee](https://github.com/ashygee)
- [@emilybrick](https://github.com/emilybrick)
- [@jonrohan](https://github.com/jonrohan)
- [@shawnbot](https://github.com/shawnbot)
- [@vdepizzol](https://github.com/vdepizzol)

# 12.5.0

### :rocket: Enhancement
- Add `.Toast` component [#831](https://github.com/primer/css/pull/831)
- Add `.SideNav` component [#827](https://github.com/primer/css/pull/827)
- Add `.SelectMenu` component [#808](https://github.com/primer/css/pull/808)
- Add `font-display: swap` to `@font-face` declarations [#780](https://github.com/primer/css/pull/780)
- Add `flex-grow-0`, `flex-order-[1,2,none]` and `width-auto` utilities [#763](https://github.com/primer/css/pull/763)
- Change default for `$marketing-font-path` to `/fonts/` [#837](https://github.com/primer/css/pull/837)

### :bug: Bug Fix
- Improve monospaced font on Chrome Android [#812](https://github.com/primer/css/pull/812)

### :memo: Documentation
- Add multi-word naming conventions to BEM docs [#836](https://github.com/primer/css/pull/836)
- Color system docs updates [#811](https://github.com/primer/css/pull/811)
- Color utility table tweaks [#842](https://github.com/primer/css/pull/842)
- Fix markdown typos in Contributing docs page [#846](https://github.com/primer/css/pull/846)

### Committers
- [@ampinsk](https://github.com/ampinsk)
- [@emilybrick](https://github.com/emilybrick)
- [@emplums](https://github.com/emplums)
- [@jonrohan](https://github.com/jonrohan)
- [@shawnbot](https://github.com/shawnbot)
- [@simurai](https://github.com/simurai)
- [@skullface](https://github.com/skullface)
- [@vdepizzol](https://github.com/vdepizzol)

# 12.4.1

### :bug: Bug fixes
- Fix [#822](https://github.com/primer/css/issues/822) (`.border-dashed` issues) via [#824](https://github.com/primer/css/issues/824)

### :memo: Documentation
- Typos fixed in [#802](https://github.com/primer/css/issues/802) (thank you, [@The-Compiler](https://github.com/The-Compiler)!)
- Nav updates [#803](https://github.com/primer/css/issues/803)
- Fix tables of contents [#762](https://github.com/primer/css/issues/762)
- Add deprecation warning for `.btn-purple`, due to disappear in 13.0.0 via [#736](https://github.com/primer/css/issues/736)
- Lots more documentation updates in [#814](https://github.com/primer/css/issues/814)

### :house: Internal
- Decommission `primer/deploy` [#809](https://github.com/primer/css/issues/809)

### Committers
- [@emplums](https://github.com/emplums)
- [@shawnbot](https://github.com/shawnbot)
- [@simurai](https://github.com/simurai)
- [@The-Compiler](https://github.com/The-Compiler)


# 12.4.0

### :rocket: Enhancement
- More responsive border utilities [#775](https://github.com/primer/css/pull/775)
- Add `overflow: visible` utilities [#798](https://github.com/primer/css/pull/798)
- Add `yellow` color utilities that will replace `pending` [#737](https://github.com/primer/css/pull/737)

### :bug: Bug Fix
- Fix Ruby Sass compiler warnings by quoting keys in `$hue-maps` declaration [#794](https://github.com/primer/css/pull/794)

### :house: Internal
- Remove `test-all-modules` scripts and old monorepo test scripts [#795](https://github.com/primer/css/pull/795)
- Resolve all but one ([#796](https://github.com/primer/css/pull/796)) vulnerability in npm dev dependencies [#797](https://github.com/primer/css/pull/797)

### Committers
- [@broccolini](https://github.com/broccolini)
- [@shawnbot](https://github.com/shawnbot)
- [@simurai](https://github.com/simurai)

# 12.3.1

### 🐛 Bug Fix
- Add `aria-selected="true"` support to tabbed navigation styles to play nicely with [`<tab-container>`](https://github.com/github/tab-container-element)

### 🏠 Internal
- Resolve the vulnerability alert with `tar@<4.4.2` in [CVE-2018-20834](https://nvd.nist.gov/vuln/detail/CVE-2018-20834)

### Committers
- [@shawnbot](https://github.com/shawnbot)

# 12.3.0

### :rocket: Enhancement
- More color utilities! [#760](https://github.com/primer/css/pull/760) ([@shawnbot](https://github.com/shawnbot))
- Add pink to Primer! 💖🌸💕🌷💞🎀💗🌺💝 [#776](https://github.com/primer/css/pull/776), [#778](https://github.com/primer/css/pull/778) ([@emplums](https://github.com/emplums)))

### :house: Internal
- Update storybook [#777](https://github.com/primer/css/pull/777) ([@emplums](https://github.com/emplums))
- Add bundle size report [#772](https://github.com/primer/css/pull/772) ([@shawnbot](https://github.com/shawnbot))

### :memo: Documentation
- Update Primer references to Primer CSS [#771](https://github.com/primer/css/pull/771) ([@emplums](https://github.com/emplums))
- Add Edit on GitHub links to docs [#770](https://github.com/primer/css/pull/770) ([@emplums](https://github.com/emplums))
- Anchor Link in Docs [#768](https://github.com/primer/css/pull/768) ([@emplums](https://github.com/emplums))

### :house: Internal
- Update RELEASING [#757](https://github.com/primer/css/pull/757) ([@simurai](https://github.com/simurai))

### Committers
- [@emplums](https://github.com/emplums)
- [@shawnbot](https://github.com/shawnbot)
- [@simurai](https://github.com/simurai)

# 12.2.3

### :bug: Bug Fix
- Remove references to a non-existent `Progress-value` class https://github.com/primer/css/pull/751

### :house: Internal
- Upgrade stylelint config https://github.com/primer/css/pull/753

#### Committers: 1
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 12.2.2

### :bug: Bug Fix
- Fix hide utilities when toggling between breakpoints [#746](https://github.com/primer/css/pull/746)

### :house: Internal
- Prevent Storybook publish failures from breaking builds on `master` [#728](https://github.com/primer/css/pull/728)
- Upgrade to [cssstats v3.3.0](https://github.com/cssstats/cssstats/releases/tag/v3.3.0), which fixes our selector stats JSON files

#### Committers: 2
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Simurai ([simurai](https://github.com/simurai))

# 12.2.1

### :bug: Bug Fix
- Fix source order of directional border utilities [#727](https://github.com/primer/css/pull/727)
- Fix UnderlineNav selected border width [#733](https://github.com/primer/css/pull/733)

### :memo: Documentation
- Fix changelog committers listings for versions 12.0.1 and 12.0.2 [#729](https://github.com/primer/css/pull/729)
- Fix code examples from being cut off [#725](https://github.com/primer/css/pull/725)

#### Committers: 2
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Simurai ([simurai](https://github.com/simurai))

# 12.2.0

### :rocket: Enhancement
- Add more `.border-white-fade` utilities [#713](https://github.com/primer/css/pull/713)

### :nail_care: Polish
- Fix `<details>` spacing [#675](https://github.com/primer/css/pull/675)

### :bug: Bug Fixes
- Accessibility fixes for marketing buttons [#716](https://github.com/primer/css/pull/716)

### :memo: Documentation
- Fix scrolling of code examples [#717](https://github.com/primer/css/pull/717)

### :house: Internal
- Fix `npm link` ([#715](https://github.com/primer/css/issue/715)) by removing `prepare` npm script [#718](https://github.com/primer/css/pull/718)

#### Committers: 4
- Diana Mounter ([broccolini](https://github.com/broccolini))
- Max Stoiber ([mxstbr](https://github.com/mxstbr))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Simurai ([simurai](https://github.com/simurai))

# 12.1.1

### :bug: Bug Fix
- Remove UI from font file path [#709](https://github.com/primer/css/pull/709)

### :memo: Documentation
- Removes sync functionality from docs [#710](https://github.com/primer/css/pull/710)

### :house: Internal
- Fixes Storybook [#707](https://github.com/primer/css/pull/707) and [#711](https://github.com/primer/css/pull/711)

#### Committers: 3
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Emily Plummer ([emplums](https://github.com/emplums))

# 12.1.0

### :rocket: Enhancement
- Per-axis overflow utilities [#701](https://github.com/primer/css/pull/701)
- Add `0` to responsive marketing positioning utilities (`top-lg-0`, et al) [#697](https://github.com/primer/css/pull/697)
- Add negative offset utilities to marketing/utilities/layout [#639](https://github.com/primer/css/pull/639)

### :memo: Documentation
- Fix changelog committers list for 12.0.2 (accidentally listed under 12.0.1)

### :house: Internal
- Remove a bunch of unused dev dependencies [#703](https://github.com/primer/css/pull/703)
- Update `script/selector-diff-report` to compare against `@primer/css` (not `primer`!)

#### Committers: 3
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Tyson Rosage ([trosage](https://github.com/trosage))

# 12.0.2

### :bug: Bug fix
- Restore missing marketing padding utilities [#695](https://github.com/primer/css/pull/695)

### :memo: Documentation
- Explain why `.tooltipped` should be used sparingly [#676](https://github.com/primer/css/pull/676)
- Fix trailing slash errors in Next.js [#681](https://github.com/primer/css/pull/681)
- Add static assets to Now deployments [#687](https://github.com/primer/css/pull/687)
- Shiny new social and README header by @ashygee [#689](https://github.com/primer/css/pull/689)

### :house: Internal
- Remove `postversion` script from `package.json` and update the [PR template](https://github.com/primer/css/blob/master/RELEASING.md#in-this-repo)
- Rename InterUI font to "Inter", per [inter v3.3](https://github.com/rsms/inter/releases/tag/v3.3) [#696](https://github.com/primer/css/pull/696)

#### Committers: 4
- Ash Guillaume ([ashygee](https://github.com/ashygee))
- David Graham ([dgraham](https://github.com/dgraham))
- Mu-An Chiou ([muan](https://github.com/muan))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 12.0.1

### :bug: Bug Fix
- Add missing `h000-mktg` class [#667](https://github.com/primer/css/pull/667)
- Fix UnderlineNav overflow issues [#684](https://github.com/primer/css/pull/684)
- Fix double borders on Box-header [#686](https://github.com/primer/css/pull/686)

### :house: Internal
- Add `postversion` npm script that commits `package.json` and `package-lock.json` with consistent commit messages (`chore: v<version>`)

#### Committers: 2
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 12.0.0

:rotating_light: **Starting with version 12.0.0, the `primer` package is now known as `@primer/css`**. See [MIGRATING.md](https://github.com/primer/css/tree/master/MIGRATING.md) for more info.

#### :boom: Breaking Change
* [#666](https://github.com/primer/css/pull/666) Reorganize into a single `@primer/css` package ([@shawnbot](https://github.com/shawnbot))

#### Committers: 2
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))

# 11.0.0

#### :boom: Breaking Change
* [#438](https://github.com/primer/primer/pull/438) Remove `primer-page-sections` package. ([@sophshep](https://github.com/sophshep))
* [#439](https://github.com/primer/primer/pull/439) Remove `primer-page-headers` package. ([@sophshep](https://github.com/sophshep))
* [#440](https://github.com/primer/primer/pull/440) Remove `primer-tables` package. ([@sophshep](https://github.com/sophshep))
* [#459](https://github.com/primer/primer/pull/459) Move responsive position utilities from marketing to core. ([@sophshep](https://github.com/sophshep))
* [#656](https://github.com/primer/primer/pull/656) Remove colorizeTooltip mixin. ([@shawnbot](https://github.com/shawnbot))
* [#657](https://github.com/primer/primer/pull/657) Remove `BtnGroup-form` class. ([@shawnbot](https://github.com/shawnbot))
* [#658](https://github.com/primer/primer/pull/658) Remove `.avatar-stack` in favor of `.AvatarStack`. ([@shawnbot](https://github.com/shawnbot))

#### :rocket: Enhancement
* [#583](https://github.com/primer/primer/pull/583) Updates to Marketing Typography. ([@sophshep](https://github.com/sophshep))
* [#660](https://github.com/primer/primer/pull/660) Add `$marketing-font-path`. ([@shawnbot](https://github.com/shawnbot))
* [#661](https://github.com/primer/primer/pull/661) Spacer variable refactor. ([@shawnbot](https://github.com/shawnbot))
* [#663](https://github.com/primer/primer/pull/663) Add deprecation warning for column grid classes and add `container-sm` utility class. ([@jonrohan](https://github.com/jonrohan))

#### :bug: Bug Fix
* [#654](https://github.com/primer/primer/pull/654) Fix typo ("Chroma") in `primer-base` comment. ([@Jiang-Xuan](https://github.com/Jiang-Xuan))
* [#655](https://github.com/primer/primer/pull/655) Fix typo ("conditonally") in `docs/src/SideNav.js` comment. ([@0xflotus](https://github.com/0xflotus))

### :house: Internal
* [#659](https://github.com/primer/primer/pull/659) Generate CSS selector diff report on Travis. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 4
- 0xflotus ([0xflotus](https://github.com/0xflotus))
- Jiang-Xuan ([Jiang-Xuan](https://github.com/Jiang-Xuan))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Sophie Shepherd ([sophshep](https://github.com/sophshep))

# 10.10.5

#### :bug: Bug Fix
* [#650](https://github.com/primer/primer/pull/650) Fix border radius edge utility specificity. ([@shawnbot](https://github.com/shawnbot))

#### :memo: Documentation
* [#649](https://github.com/primer/primer/pull/649) Sandboxed code examples. ([@shawnbot](https://github.com/shawnbot))

#### :house: Internal
- Only check links on Travis if `[check-links]` is included in the commit message
- a5658d3 Run `now alias` without the branch name on merge to `master`

#### Committers: 1
- Shawn Allen ([shawnbot](https://github.com/shawnbot))


# 10.10.4

#### :memo: Documentation
* [#642](https://github.com/primer/primer/pull/642) docs: add Ash's new header illustration. ([@shawnbot](https://github.com/shawnbot))

#### :house: Internal
* [#641](https://github.com/primer/primer/pull/641) test(docs): improve style guide URL path test. ([@shawnbot](https://github.com/shawnbot))
* [#635](https://github.com/primer/primer/pull/635) docs: Releases link, Status key page move. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 1
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 10.10.3

#### :memo: Documentation
* [#632](https://github.com/primer/primer/pull/632) Happy new year! ([@shawnbot](https://github.com/shawnbot))
* [#626](https://github.com/primer/primer/pull/626) Branch deployment, docs for the docs. ([@shawnbot](https://github.com/shawnbot))
* [#616](https://github.com/primer/primer/pull/616) Start up the docs directory. ([@shawnbot](https://github.com/shawnbot))

#### :house: Internal
* [#631](https://github.com/primer/primer/pull/631) Docs release fixes. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 2
- Emily Brick ([emilybrick](https://github.com/emilybrick))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 10.10.2

#### :memo: Documentation
* [#614](https://github.com/primer/primer/pull/614) fix broken border-radius helper example. ([@joelhawksley](https://github.com/joelhawksley))

#### :house: Internal
* [#615](https://github.com/primer/primer/pull/615) pin npm-run-all@4.1.5. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 2
- Joel Hawksley ([joelhawksley](https://github.com/joelhawksley))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 10.10.1

#### :memo: Documentation
* [#606](https://github.com/primer/primer/pull/606) Fix for Progress Broken Package Link. ([@emilybrick](https://github.com/emilybrick))

#### :house: Internal
* [#608](https://github.com/primer/primer/pull/608) Update releasing docs. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 2
- Emily Brick ([emilybrick](https://github.com/emilybrick))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 10.10.0

#### :rocket: Enhancement
* [#573](https://github.com/primer/primer/pull/573) Add Progress component. ([@emilybrick](https://github.com/emilybrick))
* [#561](https://github.com/primer/primer/pull/561) Add HTML `hidden` attribute docs, increase `[hidden]` selector specificity. ([@shawnbot](https://github.com/shawnbot) h/t @jonrohan)

#### :bug: Bug Fix
* [#604](https://github.com/primer/primer/pull/604) Fix Button group focus ring z-index issues. ([@shawnbot](https://github.com/shawnbot))
* [#570](https://github.com/primer/primer/pull/570) Make `.blankslate-narrow` responsive. ([@crhallberg](https://github.com/crhallberg))
* [#591](https://github.com/primer/primer/pull/591) Add fs-extra to `primer-module-build.dependencies`. ([@shawnbot](https://github.com/shawnbot))

#### :memo: Documentation
* [#585](https://github.com/primer/primer/pull/585) Improve contributing docs and add DEVELOP.md. ([@shawnbot](https://github.com/shawnbot))

#### :house: Internal
* [#597](https://github.com/primer/primer/pull/597) Fix primerize, add "fresh" run-script, etc. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 3
- Chris Hallberg ([crhallberg](https://github.com/crhallberg))
- Emily Brick ([emilybrick](https://github.com/emilybrick))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 10.9.0
#### :rocket: Enhancement
* [#586](https://github.com/primer/primer/pull/586) Hiding .Counter component when it's empty.. ([@jonrohan](https://github.com/jonrohan))
* [#545](https://github.com/primer/primer/pull/545) Simplify responsive utilities with $responsive-variants. ([@shawnbot](https://github.com/shawnbot))
* [#557](https://github.com/primer/primer/pull/557) Add !important to [hidden]. ([@muan](https://github.com/muan))

#### :memo: Documentation
* [#580](https://github.com/primer/primer/pull/580) Remove invalid button classes. ([@shawnbot](https://github.com/shawnbot))

#### :house: Internal
* [#581](https://github.com/primer/primer/pull/581) Use code-blocks. ([@shawnbot](https://github.com/shawnbot))
* [#530](https://github.com/primer/primer/pull/530) Adding user details to storybook publish script. ([@jonrohan](https://github.com/jonrohan))
* [#579](https://github.com/primer/primer/pull/579) Upgrade to lerna@2.11, rebuild package-lock. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 5
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- Mickaël Derriey ([mderriey](https://github.com/mderriey))
- Mu-An Chiou ([muan](https://github.com/muan))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Sophie Shepherd ([sophshep](https://github.com/sophshep))

# 10.8.1
#### :bug: Bug Fix
* [#554](https://github.com/primer/primer/pull/554) Fixes peer dependency issues ([@emplums](https://github.com/emplums))

#### :memo: Documentation
* [#554](https://github.com/primer/primer/pull/554) Updates releasing documentation ([@emplums](https://github.com/emplums))

#### :rocket: Enhancement
* [#555](https://github.com/primer/primer/pull/555) Add version check to CI ([@shawnbot](https://github.com/shawnbot))

# 10.8.0
#### :rocket: Enhancement
* [#525](https://github.com/primer/primer/pull/525) Add $spacer-0 alias. ([@AustinPaquette](https://github.com/AustinPaquette))
* [#522](https://github.com/primer/primer/pull/522) Add .BtnGroup-parent, deprecate .BtnGroup-form. ([@muan](https://github.com/muan))
* [#544](https://github.com/primer/primer/pull/544) Add lh-0 utility class. ([@shawnbot](https://github.com/shawnbot))
* [#548](https://github.com/primer/primer/pull/548) Add text underline utility. ([@AustinPaquette](https://github.com/AustinPaquette))
* [#549](https://github.com/primer/primer/pull/549) Add .user-select-none utility class. ([@AustinPaquette](https://github.com/AustinPaquette))

#### :memo: Documentation
* [#528](https://github.com/primer/primer/pull/528) Update release docs. ([@emplums](https://github.com/emplums))

#### Committers: 3
- Austin Paquette ([AustinPaquette](https://github.com/AustinPaquette))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Mu-An Chiou ([@muan](https://github.com/muan))
- Emily Plummer ([@emplums](https://github.com/emplums))

# 10.7.0

#### :nail_care: Polish
* [#511](https://github.com/primer/primer/pull/511)  change double quotes to single quotes in Avatar stack stories. ([@AustinPaquette](https://github.com/AustinPaquette))

#### :memo: Documentation
* [#520](https://github.com/primer/primer/pull/520) Update issue templates. ([@broccolini](https://github.com/broccolini))
* [#516](https://github.com/primer/primer/pull/516) Fix modules/primer-product/README.md. ([@9585999](https://github.com/9585999))
* [#513](https://github.com/primer/primer/pull/513) Deleting the dev branch workflow instructions. ([@jonrohan](https://github.com/jonrohan))
* [#507](https://github.com/primer/primer/pull/507) Moving the color docs to the style guide. ([@jonrohan](https://github.com/jonrohan))

#### :house: Internal
* [#517](https://github.com/primer/primer/pull/517) Modifying notify script to publish from each package. ([@jonrohan](https://github.com/jonrohan))
* [#515](https://github.com/primer/primer/pull/515) Auto publish storybook. ([@jonrohan](https://github.com/jonrohan))
* [#510](https://github.com/primer/primer/pull/510) [WIP] Patch release 10.6.2. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 5
- Austin Paquette ([AustinPaquette](https://github.com/AustinPaquette))
- Diana Mounter ([broccolini](https://github.com/broccolini))
- DieGOs ([9585999](https://github.com/9585999))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))

# 10.6.1

#### :bug: Bug Fix
* [#506](https://github.com/primer/primer/pull/506) Fix white border on last avatar in AvatarStack (take two). ([@shawnbot](https://github.com/shawnbot))
* [#501](https://github.com/primer/primer/pull/501) Set different z-index for .details-overlay. ([@muan](https://github.com/muan))

#### Committers: 2
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- [muan](https://github.com/muan)


# 10.6.0

#### :bug: Bug Fix
* [#491](https://github.com/primer/primer/pull/491) Add `backface-visibility` to `.hover-grow`. ([@brandonrosage](https://github.com/brandonrosage))

#### :memo: Documentation
* [#490](https://github.com/primer/primer/pull/490) Add release documentation. ([@emplums](https://github.com/emplums))

#### :house: Internal
* [#475](https://github.com/primer/primer/pull/475) Import primer-module-build to the monorepo. ([@shawnbot](https://github.com/shawnbot))
* [#479](https://github.com/primer/primer/pull/479) Add "scoreboard" test suite. ([@shawnbot](https://github.com/shawnbot))

#### Committers: 4
- Brandon Rosage ([brandonrosage](https://github.com/brandonrosage))
- Emily ([emplums](https://github.com/emplums))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- [muan](https://github.com/muan)


# 10.5.0

#### :rocket: Enhancement
* [#487](https://github.com/primer/primer/pull/487) Import Pagination Component. ([@emplums](https://github.com/emplums))
* [#474](https://github.com/primer/primer/pull/474) Add text-mono utility class. ([@emplums](https://github.com/emplums))
* [#456](https://github.com/primer/primer/pull/456) Adding height-fit utility class. ([@jonrohan](https://github.com/jonrohan))

#### :bug: Bug Fix
* [#465](https://github.com/primer/primer/pull/465) Fix Popover--right-bottom caret positioning. ([@shawnbot](https://github.com/shawnbot))
* [#458](https://github.com/primer/primer/pull/458) Fix broken pointer from packages to modules. ([@tysongach](https://github.com/tysongach))

#### :memo: Documentation
* [#486](https://github.com/primer/primer/pull/486) Documenting the text-inheritance color utility. ([@jonrohan](https://github.com/jonrohan))
* [#481](https://github.com/primer/primer/pull/481) Styleguide Polish. ([@emplums](https://github.com/emplums))
* [#464](https://github.com/primer/primer/pull/464) Fix markdown stories. ([@shawnbot](https://github.com/shawnbot))
* [#455](https://github.com/primer/primer/pull/455) Add colorizeTooltip deprecation warning. ([@jonrohan](https://github.com/jonrohan))
* [#452](https://github.com/primer/primer/pull/452) Update dead links in CONTRIBUTING.md. ([@agisilaos](https://github.com/agisilaos))

#### Committers: 7
- Agisilaos Tsaraboulidis ([agisilaos](https://github.com/agisilaos))
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))
- Emily ([emplums](https://github.com/emplums))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Tyson Gach ([tysongach](https://github.com/tysongach))
- [muan](https://github.com/muan)

# 10.4.0 (2018-03-14)

#### :rocket: Enhancement
* [#456](https://github.com/primer/primer/pull/456) Adding height-fit utility class. ([@jonrohan](https://github.com/jonrohan))

#### :memo: Documentation
* [#455](https://github.com/primer/primer/pull/455) Add colorizeTooltip deprecation warning. ([@jonrohan](https://github.com/jonrohan))
* [#452](https://github.com/primer/primer/pull/452) Update dead links in CONTRIBUTING.md. ([@agisilaos](https://github.com/agisilaos))

#### Committers: 3
- Agisilaos Tsaraboulidis ([agisilaos](https://github.com/agisilaos))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- [muan](https://github.com/muan)

# 10.3.0 (2018-01-17)

#### :rocket: Enhancement
* [#426](https://github.com/primer/primer/pull/426) Add em spacer variables. ([@broccolini](https://github.com/broccolini))
* [#430](https://github.com/primer/primer/pull/430) Increase input font-size to 16px on mobile. ([@broccolini](https://github.com/broccolini))

#### :bug: Bug Fix
* [#416](https://github.com/primer/primer/pull/416) Point style field to build file in subhead component. ([@muan](https://github.com/muan))
* [#424](https://github.com/primer/primer/pull/424) Add missing $spacer-12 in $marketingSpacers variable. ([@gladwearefriends](https://github.com/gladwearefriends))

#### :nail_care: Polish
* [#418](https://github.com/primer/primer/pull/418) Button color contrast improvements. ([@broccolini](https://github.com/broccolini))

#### :memo: Documentation
* [#427](https://github.com/primer/primer/pull/427) Adding stories from markdown for the other modules that didn't have any stories. ([@jonrohan](https://github.com/jonrohan))

#### :house: Internal
* [#420](https://github.com/primer/primer/pull/420) Update licenses to 2018 🎊. ([@jonrohan](https://github.com/jonrohan))

#### Committers: 4
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))
- Diana Mounter ([broccolini](https://github.com/broccolini))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- [muan](https://github.com/muan)

# 10.2.0 (2017-12-11)

#### :rocket: Enhancement
* [#376](https://github.com/primer/primer/pull/376) Extend spacing scale for marketing. ([@gladwearefriends](https://github.com/gladwearefriends))
* [#409](https://github.com/primer/primer/pull/409) Add Sass key to package.json. ([@broccolini](https://github.com/broccolini))
* [#358](https://github.com/primer/primer/pull/358) automatically style first and last breadcrumb. ([@gronke](https://github.com/gronke))
* [#394](https://github.com/primer/primer/pull/394) Point style field to built css. ([@koddsson](https://github.com/koddsson))

#### :memo: Documentation
* [#411](https://github.com/primer/primer/pull/411) Updates to stylelint package links/docs for new structure. ([@jonrohan](https://github.com/jonrohan))

#### Committers: 4
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))
- Diana Mounter ([broccolini](https://github.com/broccolini))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- Kristján Oddsson ([koddsson](https://github.com/koddsson))
- Stefan Grönke ([gronke](https://github.com/gronke))

# 10.1.0 (2017-11-15)

#### :rocket: Enhancement
* [#385](https://github.com/primer/primer/pull/385) New Avatar stack. ([@califa](https://github.com/califa) & [@sophshep](https://github.com/sophshep))
* [#404](https://github.com/primer/primer/pull/404) Tooltip component updates ([@broccolini](https://github.com/broccolini))

#### :memo: Documentation
* [#405](https://github.com/primer/primer/pull/405) Add deprecation warning for `.avatar-stack`. ([@jonrohan](https://github.com/jonrohan))
* [#391](https://github.com/primer/primer/pull/391) Update shields.io url to https. ([@NuttasitBoonwat](https://github.com/NuttasitBoonwat))

#### Committers: 5
- Diana Mounter ([broccolini](https://github.com/broccolini))
- Joel Califa ([califa](https://github.com/califa))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- Sophie Shepherd ([sophshep](https://github.com/sophshep))
- [NuttasitBoonwat](https://github.com/NuttasitBoonwat)

# 10.0.1 (2017-11-14)

#### :bug: Bug Fix

* Fixing `peerDependencies` to be greater than equal to versions. Fixing version mismatch with buttons and box.

# 10.0.0 (2017-11-13)

#### :boom: Breaking Change
* [#395](https://github.com/primer/primer/pull/395) Renaming primer-css to primer. ([@jonrohan](https://github.com/jonrohan))
* [#379](https://github.com/primer/primer/pull/379) Deprecating primer-cards and form-cards. ([@jonrohan](https://github.com/jonrohan))
* [#336](https://github.com/primer/primer/pull/336) Move `primer-breadcrumbs` from marketing to core ([@jonrohan]((https://github.com/jonrohan))

#### :rocket: Enhancement
* [#371](https://github.com/primer/primer/pull/371) Add .details-reset. ([@muan](https://github.com/muan))
* [#375](https://github.com/primer/primer/pull/375) New utilities & docs - fade out, hover grow, border white fade, responsive positioning, and circle. ([@sophshep](https://github.com/sophshep))
* [#383](https://github.com/primer/primer/pull/383) Add 'Popover' component. ([@brandonrosage](https://github.com/brandonrosage))
* [#377](https://github.com/primer/primer/pull/377) Refactor and add underline nav component. ([@ampinsk](https://github.com/ampinsk))
* [#337](https://github.com/primer/primer/pull/337) Add marketing buttons to primer-marketing. ([@gladwearefriends](https://github.com/gladwearefriends))
* [#342](https://github.com/primer/primer/pull/342) Add Subhead component. ([@shawnbot](https://github.com/shawnbot))
* [#341](https://github.com/primer/primer/pull/341) Add branch-name component from github/github. ([@shawnbot](https://github.com/shawnbot))

#### :bug: Bug Fix
* [#360](https://github.com/primer/primer/pull/360) Remove ::before ::after padding hack on markdown. ([@jonrohan](https://github.com/jonrohan))
* [#320](https://github.com/primer/primer/pull/320) Remove -webkit-text-decoration-skip override. ([@antons](https://github.com/antons))
* [#359](https://github.com/primer/primer/pull/359) Change markdown li break to handle Safari 10.x user stylesheet bug. ([@feministy](https://github.com/feministy))
* [#388](https://github.com/primer/primer/pull/388) Button border-radius fix to override Chroma 62. ([@broccolini](https://github.com/broccolini))
* [#307](https://github.com/primer/primer/pull/307) Do not suppress opacity transition for tooltipped-no-delay. ([@astorije](https://github.com/astorije))

#### :house: Internal
* [#396](https://github.com/primer/primer/pull/396) Use lerna-changelog to generate a changelog. ([@jonrohan](https://github.com/jonrohan))
* [#382](https://github.com/primer/primer/pull/382) Update Button docs. ([@JasonEtco](https://github.com/JasonEtco))
* [#390](https://github.com/primer/primer/pull/390) Updating `storiesFromMarkdown` to read in rails Octicons helper and replace with react component. ([@jonrohan](https://github.com/jonrohan))
* [#389](https://github.com/primer/primer/pull/389) Publish alpha release any time we're not on a release branch or master. ([@jonrohan](https://github.com/jonrohan))
* [#384](https://github.com/primer/primer/pull/384) Add test to check for the current year in the license and source. ([@jonrohan](https://github.com/jonrohan))
* [#374](https://github.com/primer/primer/pull/374) Improve Pull Request template. ([@agisilaos](https://github.com/agisilaos))

#### Committers: 13
- Agisilaos Tsaraboulidis ([agisilaos](https://github.com/agisilaos))
- Amanda Pinsker ([ampinsk](https://github.com/ampinsk))
- Anton Sotkov ([antons](https://github.com/antons))
- Brandon Rosage ([brandonrosage](https://github.com/brandonrosage))
- Catherine Bui ([gladwearefriends](https://github.com/gladwearefriends))
- Diana Mounter ([broccolini](https://github.com/broccolini))
- Jason Etcovitch ([JasonEtco](https://github.com/JasonEtco))
- Jon Rohan ([jonrohan](https://github.com/jonrohan))
- Jérémie Astori ([astorije](https://github.com/astorije))
- Mu-An ✌️ Chiou ([muan](https://github.com/muan))
- Shawn Allen ([shawnbot](https://github.com/shawnbot))
- Sophie Shepherd ([sophshep](https://github.com/sophshep))
- liz abinante! ([feministy](https://github.com/feministy))

**Special thanks to @shaharke for transferring ownership of the Primer npm package to us so that we could make the rename  happen!** :heart:

# 9.6.0

### Added
- Storybook. We've added a storybook prototyping environment for testing components in seclusion. To start the server run `npm start`
- Adding yeoman generator for creating a primer module. `generator-primer-module`
- Importing `stylelint-config-primer` from https://github.com/primer/stylelint-config-primer/ into monorepo.
- Importing `stylelint-selector-no-utility` from https://github.com/primer/stylelint-selector-no-utility into monorepo.

### Changes
- Deployment and publishing scripts refinements.

# 9.5.0

### Added
- It's now possible to style `<summary>` elements as buttons and have them appear in the active/selected state when the enclosing [`<details>` element](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/details) is open. #346

### Changes
- Updates our release candidate versioning logic so that prerelease increments are done on a per-module basis, fixing #350.

# 9.4.0

### Added
- Add `v-align-baseline` class to `primer-utilities` #324
- Add deprecation warnings for `primer-cards` and `primer-forms/lib/form-validation.scss` #347 (these will be removed in v10.0.0)

### Changes
- Update npm metadata for `primer`, `primer-core`, `primer-product`, and `primer-marketing` #328
- Remove `HEAD` heading from the changelog #327

# 9.3.0

## Added
- Docs for `primer-layout` (grid), `primer-support`, `primer-utilities`, and `primer-marketing-utilities`
- Primer keys for `category` and `module_type` to `package.json` (for use in documentation and gathering stats)

## Changes
- Removes `docs` from `gitignore`
- Removes the `^` from all dependencies so that we can publish exact versions
- Consolidates release notes from various sources into one changelog located in `/modules/primer/CHANGELOG.md`

# 9.2.0

## Added

- Add `test-docs` npm script in each module to check that every CSS class is documented (or at least mentioned) in the module's own markdown docs

## Changes

- Remove per-module configurations (`.gitignore`, `.postcss.json`, `.stylelintrc.json`) and `CHANGELOG.md` files in #284
- Replace most static `font-size`, `font-weight`, and `line-height` CSS property values with their [SCSS variable equivalents](https://github.com/primer/primer/blob/c9ea37316fbb73c4d9931c52b42bc197260c0bf6/modules/primer-support/lib/variables/typography.scss#L12-L33) in #252
- Refactor CI scripts to use Travis conditional deployment for release candidate and final release publish steps in #290

# 9.1.1

This release updates primer modules to use variables for spacing units instead of pixel values.

## Changes

- primer-alerts: 1.2.0 => 1.2.1
- primer-avatars: 1.1.0 => 1.1.1
- primer-base: 1.2.0 => 1.2.1
- primer-blankslate: 1.1.0 => 1.1.1
- primer-box: 2.2.0 => 2.2.1
- primer-breadcrumb: 1.1.0 => 1.1.1
- primer-buttons: 2.1.0 => 2.1.1
- primer-cards: 0.2.0 => 0.2.1
- primer-core: 6.1.0 => 6.1.1
- primer-css: 9.1.0 => 9.1.1
- primer-forms: 1.1.0 => 1.1.1
- primer-labels: 1.2.0 => 1.2.1
- primer-layout: 1.1.0 => 1.1.1
- primer-markdown: 3.4.0 => 3.4.1
- primer-marketing-type: 1.1.0 => 1.1.1
- primer-marketing-utilities: 1.1.0 => 1.1.1
- primer-marketing: 5.1.0 => 5.1.1
- primer-navigation: 1.1.0 => 1.1.1
- primer-page-headers: 1.1.0 => 1.1.1
- primer-page-sections: 1.1.0 => 1.1.1
- primer-product: 5.1.0 => 5.1.1
- primer-support: 4.1.0 => 4.1.1
- primer-table-object: 1.1.0 => 1.1.1
- primer-tables: 1.1.0 => 1.1.1
- primer-tooltips: 1.1.0 => 1.1.1
- primer-truncate: 1.1.0 => 1.1.1
- primer-utilities: 4.4.0 => 4.4.1

# 9.1.0

This release updates our [stylelint config](/primer/stylelint-config-primer) to [v2.0.0](https://github.com/primer/stylelint-config-primer/releases/tag/v2.0.0), and to stylelint v7.13.0. Each module also now has a `lint` npm script, and there are top-level `test` and `lint` scripts that you can use to lint and test all modules in one go.

This release also includes major improvements to our Travis build scripts to automatically publish PR builds, release candidates, and the "final" versions to npm.

# 9.0.0 - Core dependency & repo urls

We discovered that `primer-core` specified and outdated version of `primer-base` in it's dependencies. The outdated version did not have `normalize.scss` included which could cause some issues. This has issue occurred during v7.0.0 when creating the new monorepo. Also fixes repo urls in `package.json` for individual packages.

See PR [#243](https://github.com/primer/primer/pull/243)

## Changes

### Primer Core v6.0.0
- Fixed `primer-base` dependency to point to latest version

**Repo urls corrected from `packages` to `modules` in:**
- primer-base v1.1.5
- primer-box v2.1.8
- primer-buttons v2.0.6
- primer-forms v1.0.13
- primer-layout v1.0.5
- primer-navigation v1.0.6
- primer-support v4.0.7
- primer-table-object v1.0.9
- primer-tooltips v1.0.2
- primer-truncate v1.0.2
- primer-utilities v4.3.5

### Primer Product v5.0.2

**Repo urls corrected from `packages` to `modules` in:**
- primer-alerts v1.1.8
- primer-avatars v1.0.2
- primer-blankslate v1.0.2
- primer-labels v1.1.6
- primer-markdown v3.3.13
- primer-support v4.0.7

### Primer Marketing v5.0.2

**Repo urls corrected from `packages` to `modules` in:**
- primer-breadcrumb v1.0.2
- primer-cards v0.1.8
- primer-marketing-support v1.0.2
- primer-marketing-type v1.0.2
- primer-marketing-utilities v1.0.2
- primer-page-headers v1.0.2
- primer-page-sections v1.0.2
- primer-support v4.0.7
- primer-tables v1.0.2

# 8.0.0 - Imports

Fixes issues with the ordering of imports in each of our meta-packages. See PR [#239](https://github.com/primer/primer/pull/239)


## Changes

### Primer Core v5.0.1
- Re-ordered imports in `index.scss` to ensure utilities come last in the cascade

### Primer Product v5.0.1
- Re-ordered imports in `index.scss` to move markdown import to end of list to match former setup in GitHub.com

### Primer Marketing v5.0.1
- Re-ordered imports in `index.scss` to ensure marketing utilities come last in the cascade

# 7.0.0 - Monorepo

In an effort to improve our publishing workflow we turned Primer into a monorepo, made this repo the source of truth for Primer by removing Primer modules from GitHub, and setup Lerna for managing multiple packages and maintaining independent versioning for all our modules.

This is exciting because:

- we can spend less time hunting down the cause of a broken build and more time focussing on making Primer more useful and robust for everyone to use
- we can be more confident that changes we publish won't cause unexpected problems on GitHub.com and many other GitHub websites that use Primer
- we no longer have files like package.json, scripts, and readme's in the GitHub app that don't really belong there
- **we can accept pull requests from external contributors** again!

See PR for more details on this change: https://github.com/primer/primer/pull/230

## Other changes:

### Primer Core v4.0.3

#### primer-support v4.0.5
- Update fade color variables to use rgba instead of transparentize color function for better Sass readability
- Update support variables and mixins to use new color variables

#### primer-layout v1.0.3
- Update grid gutter styles naming convention and add responsive modifiers
- Deprecate `single-column` and `table-column` from layout module
- Remove `@include clearfix` from responsive container classes

#### primer-utilities v4.3.3
- Add `show-on-focus` utility class for accessibility
- Update typography utilities to use new color variables
- Add `.p-responsive` class

#### primer-base v1.1.3
- Update `b` tag font weight to use variable in base styles

### Primer Marketing v4.0.3

#### primer-tables
- Update marketing table colors to use new variables


# 6.0.0
- Add `State--small` to labels module
- Fix responsive border utilities
- Added and updated typography variables and mixins; updated variables used in typography utilities; updated margin, padding, and typography readmes
- Darken `.box-shadow-extra-large` shadow
- Update `.tooltip-multiline` to remove `word-break: break-word` property
- Add `.border-purple` utility class
- Add responsive border utilities to primer-marketing
- Add `ws-normal` utility for `whitespace: normal`
- Updated syntax and classnames for `Counters` and `Labels`, moved into combined module with states.

# 5.1.0
- Add negative margin utilities
- Move `.d-flex` & `.d-flex-inline` to be with other display utility classes in `visibility-display.scss`
- Delete `.shade-gradient` in favor of `.bg-shade-gradient`
- Removed alt-body-font variable from primer-marketing
- Removed un-used `alt` typography styles from primer-marketing
- Add green border utility

# 5.0.0
- Added new border variable and utility, replaced deprecated flash border variables
- Updated variable name in form validation
- Updated `.sr-only` to not use negative margin
- Added and removed border variables and utilities
- Add filter utility to Primer Marketing
- Removed all custom color variables within Primer-marketing in favor of the new color system
- Updated style for form group error display so it is positioned properly
- Updated state closed color and text and background pending utilities
- Removed local font css file from primer-marketing/support
- Updated all color variables and replaced 579 hex refs across modules with new variables, added additional shades to start introducing a new color system which required updating nearly all primer modules
- Added layout utility `.sr-only` for creating screen reader only elements
- Added `.flex{-infix}-item-equal` utilities for creating equal width and equal height flex items.
- Added `.flex{-infix}-row-reverse` utility for reversing rows of content
- Updated `.select-menu-button-large` to use `em` units for sizing of the CSS triangle.
- Added `.box-shadow-extra-large` utility for large, diffused shadow
- Updated: removed background color from markdown body
- Updated: remove background on the only item in an avatar stack
- Added form utility `.form-checkbox-details` to allow content to be shown/hidden based on a radio button being checked
- Added form utility to override Webkit's incorrect assumption of where to try to autofill contact information

# 4.7.0
- Update primer modules to use bold variable applying `font-weight: 600`

# 4.6.0
- Added `CircleBadge` component for badge-like displays within product/components/avatars
- Added Box shadow utilities `box-shadow`, `box-shadow-medium`, `box-shadow-large`, `box-shadow-none`
- Moved visibility and display utilities to separate partial at the end of the imports list, moved flexbox to it's own partial
- Added `flex-shrink-0` to address Flexbox Safari bug
- Updated: Using spacing variables in the `.flash` component
- Updated Box component styles and documentation
- Added `.wb-break-all` utility

# 4.4.0
- Adding primer-marketing module to primer
- Added red and blue border color variables and utilities
- Updated: `$spacer-5` has been changed to `32px` from `36px`
- Updated: `$spacer-6` has been changed to `40px` from `48px`
- Deprecated `link-blue`, updated `link-gray` and `link-gray-dark`, added `link-hover-blue` - Updated: blankslate module to use support variables for sizing

# 4.3.0
- Renamed `.flex-table` to `.TableObject`
- Updated: `$spacer-1` has been changed to `4px` from `3px`
- Updated: `$spacer-2` has been changed to `6px` from `8px`
- Added: `.text-shadow-dark` & `.text-shadow-light` utilities
- Updated: Moved non-framework CSS out of Primer modules. Added `box.scss` to `primer-core`. Added `discussion-timeline.scss` to `primer-product`, and moved `blob-csv.scss` into `/primer-product/markdown` directory
- Added: Flex utilities
- Refactor: Site typography to use Primer Marketing styles
- Added: `.list-style-none` utility
- Refactor: Button groups into some cleaner CSS
- Updated: Reorganizing how we separate primer-core, primer-product, primer-marketing css


# 4.2.0
- Added: Responsive styles for margin and padding utilities, display,  float, and new responsive hide utility, and updates to make typography responsive
- Added: new container styles and grid styles with responsive options
- Added: updated underline nav styles
- Deprecate: Deprecating a lot of color and layout utilities
- Added: More type utilities for different weights and larger sizes.
- Added: Well defined browser support


# 4.1.0
- Added: [primer-markdown](https://github.com/primer/markdown) to the build
- Fixes: Pointing "style" package.json to `build/build.css` file.
- Added: Update font stack to system fonts
- Added: Updated type scale as part of system font update
- Added: `.Box` component for replacing boxed groups, simple box, and table-list styles
- Added: New type utilities for headings and line-height
- Deprecated: `vertical-middle` was replaced with `v-align-middle`.
- Added: Layout utilities for vertical alignment, overflow, width and height, visibility, and display table
- Added: Changing from font icons to SVG

# 4.0.2
- Added npm build scripts to add `build/build.css` to the npm package

# 4.0.1
- Fixed: missing primer-layout from build

# 4.0.0
- Whole new npm build system, pulling in the code from separate component repos

# 3.0.0
- Added: Animation utilities
- Added: Whitespace scale, and margin and padding utilities
- Added: Border utilities
