const fs = require('fs');
const path = require('path');

const github = require('@actions/github');
const core = require('@actions/core');
const Octokit = require('@octokit/rest');

const GITHUB_TOKEN = process.env['GITHUB_TOKEN'];
const MSI_PATH = core.getInput('msi-file');

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
  
  const octokit = new github.GitHub(GITHUB_TOKEN);
  const release = await getRelease(octokit, owner, repo, tag);

  const assetFile = fs.createReadStream(MSI_PATH);
  
  try {
    await uploadAsset(octokit, release, assetFile);
    await deleteAsset(octokit, release, owner, repo, path.basename(MSI_PATH, '.msi') + '.exe');
  } finally {
    assetFile.close()
  }
}

/**
 * @param {github.GitHub} octokit
 */
async function getRelease(octokit, owner, repo, tag) {
  const response = await octokit.repos.getReleaseByTag({ owner, repo, tag });
  return response.data;
}

/**
 * @param {github.GitHub} octokit
 * @param {Octokit.ReposGetReleaseByTagResponse} release
 */
async function uploadAsset(octokit, release, assetFile) {
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

/**
 * @param {github.GitHub} octokit
 * @param {Octokit.ReposGetReleaseByTagResponse} release
 */
async function deleteAsset(octokit, release, owner, repo, assetName) {
  for (const asset of release.assets) {
    if (asset.name == assetName) {
      await octokit.repos.deleteReleaseAsset({
        owner,
        repo,
        asset_id: asset.id
      })
    }
  }
}

async function handleError(err) {
  console.error(err);
  core.setFailed(err.message);
}
