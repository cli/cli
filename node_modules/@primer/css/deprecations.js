/*
 * This object's keys are (semver) version numbers, and the
 * values are arrays of objects each with a "selectors"
 * array and a "message" string.
 */
const versionDeprecations = {
  '13.0.0': [
    {
      selectors: ['.btn-purple'],
      message: `Please don't make purple buttons.`
    },
    {
      selectors: ['.text-pending'],
      message: `Please use the "text-yellow" class instead of "text-pending".`
    },
    {
      selectors: ['.bg-pending'],
      message: `Please use the "bg-yellow-dark" class instead of "bg-pending".`
    },
    {
      selectors: [
        '.columns',
        '.column',
        '.one-third',
        '.two-thirds',
        '.one-fourth',
        '.one-half',
        '.three-fourths',
        '.one-fifth',
        '.four-fifths'
      ],
      message: `Please use [grid classes](https://primer.style/css/objects/grid).`
    },
    {
      selectors: ['.centered'],
      message: `You can use the "mx-auto" class to center any element.`
    }
  ]
}

const {version: CURRENT_VERSION} = require('./package.json')
const semver = require('semver')

// map selectors to the version and message of their deprecation
const selectorDeprecations = new Map()
for (const [version, deps] of Object.entries(versionDeprecations)) {
  for (const {selectors, message} of deps) {
    for (const selector of selectors) {
      selectorDeprecations.set(selector, {version, message})
    }
  }
}

function isSelectorDeprecated(selector, version = CURRENT_VERSION) {
  const deprecation = selectorDeprecations.get(selector)
  return deprecation ? semver.gte(deprecation.version, version) : false
}

module.exports = {versionDeprecations, selectorDeprecations, isSelectorDeprecated}
