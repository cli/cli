const fs = require('fs');
const path = require('path');

const github = require('@actions/github');
const core = require('@actions/core');

const GITHUB_TOKEN = process.env['GITHUB_TOKEN'];
const MSI_PATH = 'gh.msi';

process.on('unhandledRejection', function(reason, _) {
  handleError(reason);
});

main().catch(handleError)

async function main() {
  if (!fs.existsSync(MSI_PATH)) {
    throw new Error(`Could not find MSI file at ${MSI_PATH}`);
  }
  const tag = github.context.ref.replace('refs/tags/', '');

  const owner = github.context.repo.owner;
  const repo = github.context.repo.repo;


  const release = await getRelease(GITHUB_TOKEN, owner, repo, tag);

  const assetFile = fs.readFileSync(MSI_PATH);

  await uploadAsset(GITHUB_TOKEN, release, assetFile);
}

async function getRelease(token, owner, repo, tag) {
  const octokit = new github.GitHub(token);
  const response = await octokit.repos.getReleaseByTag({ owner, repo, tag });
  return response.data;
}

async function uploadAsset(token, release, assetFile) {
  const octokit = new github.GitHub(token);
  await octokit.repos.uploadReleaseAsset({
    url: release.upload_url,
    file: assetFile,
    name: path.basename(MSI_PATH),
    headers: {
      'content-type': 'application/octet-stream',
      'content-length': fs.statSync(MSI_PATH).size,
    }
  });
}

async function handleError(err) {
  console.error(err);
  core.setFailed(err.message);
}
