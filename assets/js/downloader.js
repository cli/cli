// Show downloader on landing page for user's OS

var os = '';

if (navigator.appVersion.indexOf('Win') != -1) {
  os = 'windows';
}
if (navigator.appVersion.indexOf('Mac') != -1) {
  os = 'mac';
}
if (navigator.appVersion.indexOf('Linux') != -1) {
  os = 'linux'
}

Array.from(document.querySelectorAll('.download-' + os)).forEach(function(el) {
  el.classList.remove('d-none')
})
