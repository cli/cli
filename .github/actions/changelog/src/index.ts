import * as core from '@actions/core'
import * as github from '@actions/github'

const GITHUB_TOKEN = getEnvVar('GITHUB_TOKEN')

process.on('unhandledRejection', (reason: any, _: Promise<any>) =>
  handleError(reason)
)
main().catch(handleError)

async function main() {
  const title =
    github.context.payload &&
    github.context.payload.pull_request &&
    github.context.payload.pull_request.title
  core.info('title: ' + title)
  await getChangelog()
}

async function getChangelog() {
  const octokit = new github.GitHub(GITHUB_TOKEN)
  const owner = github.context.repo.owner
  const repo = github.context.repo.repo
  const path = 'changelog.json'
  const response = await octokit.repos.getContents({ owner, repo, path })
  const data = response.data
  core.info('content: ' + JSON.stringify(data))
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
