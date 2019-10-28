const core = require('@actions/core')
const github = require('@actions/github')
const fs = require('fs')
const path = require('path')
const fetch = require('node-fetch')

const GITHUB_TOKEN = getEnvVar('GITHUB_TOKEN')
const UPLOAD_GITHUB_TOKEN = getEnvVar('UPLOAD_GITHUB_TOKEN')
const INPUT_UPLOAD_OWNER_NAME = getEnvVar('INPUT_UPLOAD_OWNER_NAME')
const INPUT_UPLOAD_REPO_NAME = getEnvVar('INPUT_UPLOAD_REPO_NAME')

process.on('unhandledRejection', (reason: any, _: Promise<any>) =>
  handleError(reason)
)
main().catch(handleError)

async function main() {
  const nwo = getEnvVar('GITHUB_REPOSITORY')
  const [owner, repo] = nwo.split('/')

  try {
    // Get the release from the current repository
    const release = await getRelease(owner, repo)
    const asset = release.assets.find((a: any) => {
      return a.name.match(/_darwin_amd64.tar.gz/i)
    })
    const filePath = await downloadAsset(asset)

    // Create a new release in another repository
    const publicRelease = await createRelease(release)
    const assetUrl = await uploadAsset(publicRelease, filePath)
    core.setOutput('asset-url', assetUrl)
  } catch (error) {
    handleError(error)
  }
}

async function getRelease(owner: string, repo: string) {
  const octokit = new github.GitHub(GITHUB_TOKEN)
  const tag = getEnvVar('GITHUB_REF').replace(/refs\/tags\//, '')
  const response = await octokit.repos.getReleaseByTag({ owner, repo, tag })
  checkResponse(response)
  return response.data
}

async function createRelease(release: any) {
  const octokit = new github.GitHub(UPLOAD_GITHUB_TOKEN)

  const response = await octokit.repos.createRelease({
    owner: INPUT_UPLOAD_OWNER_NAME,
    repo: INPUT_UPLOAD_REPO_NAME,
    tag_name: release.tag_name,
    name: release.name,
    body: release.body,
    target_commish: release.target_commish,
    prerelease: release.prerelease,
    draft: false,
  })
  checkResponse(response)
  return response.data
}

async function uploadAsset(release: any, filePath: string) {
  const octokit = new github.GitHub(UPLOAD_GITHUB_TOKEN)
  const response = await octokit.repos.uploadReleaseAsset({
    url: release.upload_url,
    file: fs.readFileSync(filePath),
    name: path.basename(filePath),
    headers: {
      'content-type': 'application/gzip',
      'content-length': fs.statSync(filePath).size,
    },
  })
  checkResponse(response)
  return response.data.browser_download_url
}

async function downloadAsset(asset: any) {
  const options = {
    redirect: 'manual',
    headers: {
      Authorization: `token ${GITHUB_TOKEN}`,
      Accept: 'application/octet-stream',
    },
  }

  let response = await fetch(asset.url, options)
  // Why didn't I just let fetch handle the redirect? Because that will
  // will forwarded the Authorization header to S3 and AWS doesn't like that.
  // For more deatails check out https://github.com/octokit/rest.js/issues/967
  if (response.status === 302) {
    response = await fetch(response.headers.get('location'), {
      headers: { Accept: 'application/octet-stream' },
    })
  }

  if (response.status === 200) {
    const data = await response.buffer()
    fs.writeFileSync(asset.name, data)
    return asset.name
  } else {
    throw new Error('failed to download asset: ' + (await response.text()))
  }
}

async function handleError(err: Error) {
  console.error(err)
  core.setFailed(err.message)
}

function checkResponse(response: any) {
  if (response.status >= 200 || response.status < 300) return

  throw new Error(
    `Failed to run ${response.request.url}. Errors: ${response.errors}`
  )
}

function getEnvVar(name: string) {
  const envVar = process.env[name]
  if (!envVar) {
    throw new Error(`env var named "${name} is not set"`)
  }
  return envVar
}
