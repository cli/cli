package context

const oauthSuccessPage = `
<!doctype html>
<meta charset="utf-8">
<title>Success: GitHub CLI</title>
<style type="text/css">
body {
    color: #333;
    font-size: 14px;
    font-family: -apple-system, "Segoe UI", Helvetica, Arial, sans-serif;
    line-height: 1.5;
    max-width: 461px;
    margin: 2em auto;
    text-align: center;
}
h1 {
    color: #555;
    font-size: 22px;
    letter-spacing: 1px;
}
</style>

<body>
    <h1>Authentication successful.</h1>
    <p>
        You have completed logging into GitHub CLI.<br>
        You may now <strong>close this tab and return to the terminal</strong>.
    </p>
    <img alt="" src="https://octodex.github.com/images/daftpunktocat-guy.gif" height="461">
</body>
`
