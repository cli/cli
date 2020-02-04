module.exports = {
  parser: 'postcss-scss',
  map: {
    sourcesContent: true,
    annotation: true
  },
  plugins: {
    'postcss-node-sass': {
      includePaths: ['node_modules'],
      outputStyle: 'compressed'
    },
    'autoprefixer': {}
  }
}
