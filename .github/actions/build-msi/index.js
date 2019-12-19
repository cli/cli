const core = require('@actions/core');
const github = require('@actions/github');
const exec = require('@actions/exec');
const fs = require('fs');
const path = require('path');

process.on('unhandledRejection', function(reason, _) {
  handleError(reason);
});

function addToPath(_path) {
  console.log(`adding ${_path} to Path`);
  process.env['Path'] = _path + ';' + process.env['Path'];
}

main().catch(handleError)

async function main() {
  const exePath = core.getInput('exe');
  const version = github.context.ref.replace('refs/tags/', '');

  console.log(`Building MSI for version ${version}`);
  const wixPath = process.env['WIX'] + '\\bin';

  addToPath(wixPath);

  await go_msi(version, exePath);
}

async function go_msi(version, exePath) {
  const cwd = process.cwd();

  // go-msi was struggling with its default temp dir; it wanted something relative to the source dir
  // for some reason.
  console.log('making build path');
  const buildPath = path.join(cwd, 'build');
  fs.mkdirSync(buildPath);

  console.log('making bin path');
  const binPath = path.join(cwd, 'bin');
  fs.mkdirSync(binPath);
  console.log(`moving ${exePath} to bin/gh.exe`);
  fs.renameSync(exePath, path.join(binPath, 'gh.exe'));

  const msiPath = path.join(cwd, 'gh.msi');

  try {
    await exec.exec(
      '"C:\\Program Files\\go-msi\\go-msi.exe"',
      ['make',
       '--msi', msiPath,
       '--out', buildPath,
       '--version', version]);
    console.log(`build MSI at ${msiPath}`);
    core.setOutput('msi', msiPath);
  } catch(e) {
    core.setFailed(e.message);
  }
}

async function handleError(err) {
  console.error(err);
  core.setFailed(err.message);
}
