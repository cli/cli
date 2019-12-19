import {getInput, setOutput, setFailed} from '@actions/core'
import {context, GitHub} from '@actions/github'
import * as fs from 'fs'
import * as path from 'path'
import fetch from 'node-fetch'
import { ReposCreateReleaseResponse, ReposGetReleaseByTagResponse } from '@octokit/rest'

const GITHUB_TOKEN = getEnvVar('GITHUB_TOKEN')
const UPLOAD_GITHUB_TOKEN = getEnvVar('UPLOAD_GITHUB_TOKEN')

process.on('unhandledRejection', (reason: any, _: Promise<any>) =>
  handleError(reason)
)
main().catch(handleError)

async function main() {
  // Get the release from the current repository
  const tag = context.ref.replace('refs/tags/', '')
  const release = await getRelease(context.repo.owner, context.repo.repo, tag)

  // Create a new release in another repository
  const [targetOwner, targetRepo] = getInput('target-repo').split('/')
  let publicRelease: ReposCreateReleaseResponse | ReposCreateReleaseResponse
  try {
    publicRelease = await getRelease(targetOwner, targetRepo, tag)
  } catch (error) {
    if (error.status && error.status == 404) {
      publicRelease = await createRelease(targetOwner, targetRepo, release)
    } else {
      throw error
    }
  }

  for (const asset of release.assets) {
    const filePath = await downloadAsset(asset)
    for (const existingAsset of publicRelease.assets) {
      if (existingAsset.name == asset.name) {
        await deleteAsset(targetOwner, targetRepo, existingAsset)
      }
    }
    const assetUrl = await uploadAsset(publicRelease, filePath)
    if (asset.name.match(/macOS/) || asset.name.match(/darwin/)) {
      setOutput('asset-url', assetUrl)
    }
  }
}

async function getRelease(owner: string, repo: string, tag: string) {
  const octokit = new GitHub(GITHUB_TOKEN)
  const response = await octokit.repos.getReleaseByTag({ owner, repo, tag })
  return response.data
}

async function createRelease(owner: string, repo: string, release: ReposGetReleaseByTagResponse) {
  const octokit = new GitHub(UPLOAD_GITHUB_TOKEN)
  const response = await octokit.repos.createRelease({
    owner,
    repo,
    tag_name: release.tag_name,
    name: release.name,
    body: '',
    prerelease: release.prerelease,
    draft: false,
  })
  return response.data
}

async function uploadAsset(release: ReposGetReleaseByTagResponse | ReposCreateReleaseResponse, filePath: string) {
  const octokit = new GitHub(UPLOAD_GITHUB_TOKEN)
  const response = await octokit.repos.uploadReleaseAsset({
    url: release.upload_url,
    file: fs.readFileSync(filePath),
    name: path.basename(filePath),
    headers: {
      'content-type': 'application/octet-stream',
      'content-length': fs.statSync(filePath).size,
    },
  })
  // this seems like a bug in Rest.js types
  const url: string = (<any>response.data).browser_download_url
  return url
}

interface Asset {
  id: number;
  url: string;
  name: string;
}

async function deleteAsset(owner: string, repo: string, asset: Asset) {
  const octokit = new GitHub(UPLOAD_GITHUB_TOKEN)
  await octokit.repos.deleteReleaseAsset({
    owner,
    repo,
    asset_id: asset.id
  })
}

async function downloadAsset(asset: Asset) {
  let response = await fetch(asset.url, {
    redirect: 'manual',
    headers: {
      Authorization: `token ${GITHUB_TOKEN}`,
      Accept: 'application/octet-stream',
    },
  })

  // Why didn't I just let fetch handle the redirect? Because that will
  // will forwarded the Authorization header to S3 and AWS doesn't like that.
  // For more details check out https://github.com/octokit/rest.js/issues/967
  if (response.status === 302) {
    response = await fetch(response.headers.get('location') || '', {
      headers: { Accept: 'application/octet-stream' },
    })
  }

  if (response.status === 200) {
    const data = await response.buffer()
    fs.writeFileSync(asset.name, data)
  } else {
    throw new Error('failed to download asset: ' + (await response.text()))
  }
  return asset.name
}

async function handleError(err: Error) {
  console.error(err)
  setFailed(err.message)
}

function getEnvVar(name: string): string {
  const envVar = process.env[name]
  if (!envVar) {
    throw new Error(`env var named "${name} is not set"`)
  }
  return envVar
}
