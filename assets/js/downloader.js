// Show downloader on landing page for user's OS

$( document ).ready(function() {
  var os = '';

  if (navigator.appVersion.indexOf('Win') != -1) {
    os = 'windows';
  }
  if (navigator.appVersion.indexOf('Mac') != -1) {
    os = 'mac';
  }
  if (navigator.appVersion.indexOf("Linux") != -1) {
    os = 'linux'
  }

  $( '.download-' + os ).each(function( i, downloader ) {
    $(downloader).removeClass('d-none');
  });
});
