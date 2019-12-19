import * as core from '@actions/core'
import * as github from '@actions/github'

const GITHUB_TOKEN = getEnvVar('GITHUB_TOKEN')

interface Changelog {
  draft: string[]
  releases: { [tag: string]: string[] }
}

process.on('unhandledRejection', (reason: any, _: Promise<any>) =>
  handleError(reason)
)
main().catch(handleError)

async function main() {
  const title =
    github.context.payload &&
    github.context.payload.pull_request &&
    github.context.payload.pull_request.title
  const { changelog, sha } = await getChangelog()
  const updatedChangelog = { ...changelog, draft: [title, ...changelog.draft] }
  await commitChangelog(updatedChangelog, sha)
}

async function getChangelog() {
  const octokit = new github.GitHub(GITHUB_TOKEN)
  const owner = github.context.repo.owner
  const repo = github.context.repo.repo
  const path = 'changelog.json'
  const response = await octokit.repos.getContents({ owner, repo, path })
  const base64Content = (response.data as any).content
  const sha = (response.data as any).sha
  const content = Buffer.from(base64Content, 'base64').toString('utf8')
  return { changelog: JSON.parse(content), sha }
}

async function commitChangelog(changelog: Changelog, sha: string) {
  const octokit = new github.GitHub(GITHUB_TOKEN)
  const owner = github.context.repo.owner
  const repo = github.context.repo.repo
  const path = 'changelog.json'
  const message = 'update changelog'
  const json = JSON.stringify(changelog, null, 2)
  const content = Buffer.from(json).toString('base64')
  const response = await octokit.repos.createOrUpdateFile({
    owner,
    repo,
    path,
    message,
    content,
    sha,
  })

  if (response.status != 200) {
    throw new Error(
      'Failed to commit changelog udpates. ' + JSON.stringify(response.data)
    )
  }
}

function handleError(err: Error) {
  console.error(err)
  core.setFailed(err.message)
}

function getEnvVar(name: string): string {
  const envVar = process.env[name]
  if (!envVar) {
    throw new Error(`env var named "${name} is not set"`)
  }
  return envVar
}
