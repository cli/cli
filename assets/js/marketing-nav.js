// Attach event listners to marketing header nav
// from github/github

(function() {
const touchDevice = 'ontouchstart' in document

function compatibleDesktop() {
  return window.innerWidth > 1012
}

for (const headerMenu of document.querySelectorAll('.HeaderMenu-details')) {
  // On desktop and mobile, ensure that other menus are closed when one opens.
  headerMenu.addEventListener('toggle', onMenuToggle)
  if (!touchDevice) {
    // We can't use `mouseenter` because of Safari bug (v12.0.1).
    headerMenu.addEventListener('mouseover', onMenuHover)
    headerMenu.addEventListener('mouseleave', onMenuHover)
    // On desktop, because the menus are automatically closed on hover, disable
    // manually collapsing menus to prevent accidental interactions.

    // This is currently commented out due to a bug where dropdown links are not clickable. Awaiting a possible work around
    // headerMenu.addEventListener('click', disableMenuManualClose)
  }
}

let togglingInProgress = false
function onMenuToggle(event) {
  if (togglingInProgress) return
  togglingInProgress = true

  for (const headerMenu of document.querySelectorAll('.HeaderMenu-details')) {
    if (headerMenu === event.currentTarget) continue
    headerMenu.removeAttribute('open')
  }

  setTimeout(() => (togglingInProgress = false))
}

function onMenuHover(event) {
  const {currentTarget} = event
  if (!(currentTarget instanceof HTMLElement) || !compatibleDesktop()) {
    return
  }
  if (event.type === 'mouseover') {
    if (
      event.target instanceof Node &&
      event.relatedTarget instanceof Node &&
      currentTarget.contains(event.target) &&
      !currentTarget.contains(event.relatedTarget)
    ) {
      currentTarget.setAttribute('open', '')
    }
  } else {
    currentTarget.removeAttribute('open')
  }
}

// Toggle mobile nav
var mobileNavBtns = [].slice.call(document.querySelectorAll(".js-details-target"));
var header = document.querySelector('.Header')

mobileNavBtns.forEach(function(btn) {
  btn.addEventListener('click', function() {
    console.log('click');
    if (header.classList.contains('open')) {
      header.classList.remove('open')
    } else {
      header.classList.add('open')
    }
  })
})
}());
