GitHub takes the security of our software products and services seriously, including the open source code repositories managed through our GitHub organizations, such as [cli](https://github.com/cli).

If you believe you have found a security vulnerability in GitHub CLI, you can report it to us in one of two ways:

* Report it to this repository directly using [private vulnerability reporting][]. Such reports are not eligible for a bounty reward.

* Submit the report through [HackerOne][] to be eligible for a bounty reward.

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

Thanks for helping make GitHub safe for everyone.

  [private vulnerability reporting]: https://github.com/cli/cli/security/advisories
  [HackerOne]: https://hackerone.com/github
pipeline {
  agent any

  tools {nodejs "{your_nodejs_configured_tool_name}"}

  stages {
    stage('Install Postman CLI') {
      steps {
        sh 'curl -o- "https://dl-cli.pstmn.io/install/linux64.sh" | sh'
      }
    }

    stage('Postman CLI Login') {
      steps {
        sh 'postman login --with-api-key $POSTMAN_API_KEY'
        }
    }

    stage('Running collection') {
      steps {
        sh 'postman collection run "18162788-861dfffd-4daf-49a6-b173-c4d07611b781"'
      }
    }
  }
}
