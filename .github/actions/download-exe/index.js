const fs = require('fs');

const fetch = require('node-fetch');
const core = require('@actions/core');
const github = require('@actions/github');

const GITHUB_TOKEN = process.env['GITHUB_TOKEN'];

process.on('unhandledRejection', function(reason, _) {
  handleError(reason);
});

main().catch(handleError)

async function main() {
  const tag = github.context.ref.replace('refs/tags/', '');

  const owner = github.context.repo.owner;
  const repo = github.context.repo.repo;

  const release = await getRelease(GITHUB_TOKEN, owner, repo, tag);
  const asset = await findExeAsset(release);

  const filePath = await downloadAsset(asset);

  core.setOutput('exe', filePath);
}

async function downloadAsset(asset) {
  // SHAMELESSLY COPIED FROM COPY RELEASE ACTION
  let response = await fetch(asset.url, {
    redirect: 'manual',
    headers: {
      Authorization: `token ${GITHUB_TOKEN}`,
      Accept: 'application/octet-stream',
    },
  });

  // Why didn't I just let fetch handle the redirect? Because that will
  // will forwarded the Authorization header to S3 and AWS doesn't like that.
  // For more details check out https://github.com/octokit/rest.js/issues/967
  if (response.status === 302) {
    response = await fetch(response.headers.get('location') || '', {
      headers: { Accept: 'application/octet-stream' },
    });
  }

  if (response.status === 200) {
    const data = await response.buffer();
    fs.writeFileSync(asset.name, data);
  } else {
    throw new Error('failed to download asset: ' + (await response.text()));
  }
  return asset.name;
}

async function findExeAsset(release) {
  for (const asset of release.assets) {
    if (asset.name.endsWith('.exe')) {
      return asset;
    }
  }
  throw new Error('could not find exe in release');
}

async function getRelease(token, owner, repo, tag) {
  const octokit = new github.GitHub(token);
  const response = await octokit.repos.getReleaseByTag({ owner, repo, tag });
  return response.data;
}

async function handleError(err) {
  console.error(err);
  core.setFailed(err.message);
}
