$( document ).ready(function() {

  var commands = $('.command-header').find('.command');
  var activeCommandIndex = 1;

  function showCommand (index) {
    switch(index) {
      case 1:
        var typedHeader = new Typed('.command-header-1', {
          strings: ['gh pr status'],
          showCursor: false,
          typeSpeed: 40
        });

        var typedOutput = new Typed('.command-output-1', {
          strings: ['<span class="text-white">gh pr status</span>^1000\n `<br><strong class="text-white">Current branch</strong><br><span class="pl-3">There is no pull request associated with <span class="text-blue-light">[fix-homepage-bug]</span></span><br><br><strong class="text-white">Created by you</strong><br><span class="pl-3">You have no open pull requests</span><br><br><strong class="text-white">Requesting a code review from you</strong><br><span class="pl-3"><span style="color: yellow;">#100</span>  <span class="text-white">Fix footer on homepage</span> <span class="text-blue-light">[fix-homepage-footer]</span></span><br><span class="pl-5">- <span style="color: chartreuse;">Checks passing</span> - Approved</span>`'],
          showCursor: false,
          typeSpeed: 40
        });
        break;
      case 2:
        var typedHeader = new Typed('.command-header-2', {
          strings: ['gh issue list'],
          showCursor: false,
          typeSpeed: 40
        });
        // TODO update this terminal output
        var typedOutput = new Typed('.command-output-2', {
          strings: ['<span class="text-white">gh issue list</span>^1000\n `<br><strong class="text-white">Current branch</strong><br><span class="pl-3">There is no pull request associated with <span class="text-blue-light">[fix-homepage-bug]</span></span><br><br><strong class="text-white">Created by you</strong><br><span class="pl-3">You have no open pull requests</span><br><br><strong class="text-white">Requesting a code review from you</strong><br><span class="pl-3"><span style="color: yellow;">#100</span>  <span class="text-white">Fix footer on homepage</span> <span class="text-blue-light">[fix-homepage-footer]</span></span><br><span class="pl-5">- <span style="color: chartreuse;">Checks passing</span> - Approved</span>`'],
          showCursor: false,
          typeSpeed: 40
        });
        break;
      case 3:
        var typedHeader = new Typed('.command-header-3', {
          strings: ['gh pr create'],
          showCursor: false,
          typeSpeed: 40
        });
        // TODO update this terminal output
        var typedOutput = new Typed('.command-output-3', {
          strings: ['<span class="text-white">gh pr create</span>^1000\n `<br><strong class="text-white">Current branch</strong><br><span class="pl-3">There is no pull request associated with <span class="text-blue-light">[fix-homepage-bug]</span></span><br><br><strong class="text-white">Created by you</strong><br><span class="pl-3">You have no open pull requests</span><br><br><strong class="text-white">Requesting a code review from you</strong><br><span class="pl-3"><span style="color: yellow;">#100</span>  <span class="text-white">Fix footer on homepage</span> <span class="text-blue-light">[fix-homepage-footer]</span></span><br><span class="pl-5">- <span style="color: chartreuse;">Checks passing</span> - Approved</span>`'],
          showCursor: false,
          typeSpeed: 40
        });
        break;
      case 4:
        var typedHeader = new Typed('.command-header-4', {
          strings: ['gh pr checkout'],
          showCursor: false,
          typeSpeed: 40
        });
        // TODO update this terminal output
        var typedOutput = new Typed('.command-output-4', {
          strings: ['<span class="text-white">gh pr checkout</span>^1000\n `<br><strong class="text-white">Current branch</strong><br><span class="pl-3">There is no pull request associated with <span class="text-blue-light">[fix-homepage-bug]</span></span><br><br><strong class="text-white">Created by you</strong><br><span class="pl-3">You have no open pull requests</span><br><br><strong class="text-white">Requesting a code review from you</strong><br><span class="pl-3"><span style="color: yellow;">#100</span>  <span class="text-white">Fix footer on homepage</span> <span class="text-blue-light">[fix-homepage-footer]</span></span><br><span class="pl-5">- <span style="color: chartreuse;">Checks passing</span> - Approved</span>`'],
          showCursor: false,
          typeSpeed: 40
        });
        break;
    }
  }

  function showNextCommand () {
    // hide and clear div contents of prev command
    console.log('clearing', activeCommandIndex - 1);
    $('.command-' + (activeCommandIndex - 1)).toggleClass('d-none');
    $('.command-header-' + (activeCommandIndex - 1)).html('');
    $('.command-output-' + (activeCommandIndex - 1)).html('');

    // if at the end of index, then start at 1
    if (activeCommandIndex == commands.length + 1) {
      activeCommandIndex = 1;
    }

    $('.command-' + activeCommandIndex).toggleClass('d-none');
    showCommand(activeCommandIndex);
    activeCommandIndex += 1;

    setTimeout(function(){ showNextCommand(); }, 8000);
  }

  showNextCommand();
});
