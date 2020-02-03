var keystrokeDelay = 60  // ms between keystrokes
var outputDelay = 300    // ms before command output is shown
var cycleDelay = 8000    // ms between going to next command

var numCommands = document.querySelectorAll('.command-header .command').length

function showCommand(index) {
  if (index > numCommands) index = 1

  // hide previous commands
  Array.from(document.querySelectorAll('.command')).forEach(function(el) {
    el.classList.add('d-none')
  })

  // show current command while respecting animated ones
  Array.from(document.querySelectorAll('.command-'+index)).forEach(function(el) {
    if (el.classList.contains('type-animate-done')) return
    if (el.classList.contains('type-animate')) {
      typeAnimate(el, function() {
        var doneEl = el.nextElementSibling
        if (doneEl && doneEl.classList.contains('type-animate-done')) {
          setTimeout(function() {
            doneEl.classList.remove('d-none')
          }, outputDelay)
        }
      })
    }
    el.classList.remove('d-none')
  })

  // force "layout"
  document.querySelector('.command-header').clientLeft

  setTimeout(function() { showCommand(index+1) }, cycleDelay)
}

function typeAnimate(el, callback) {
  var chars = el.textContent.split('')
  el.textContent = ''

  var typeIndex = 1
  var interval = setInterval(function() {
    el.textContent = chars.slice(0, typeIndex++).join('')
    if (typeIndex > chars.length) {
      clearInterval(interval)
      interval = null
      callback()
    }
  }, keystrokeDelay)
}

showCommand(1)
