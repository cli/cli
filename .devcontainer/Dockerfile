# See here for image contents: https://github.com/microsoft/vscode-dev-containers/tree/v0.234.0/containers/go/.devcontainer/base.Dockerfile

# VARIANT Defined in devcontainer.json
ARG VARIANT="1.18" 
FROM mcr.microsoft.com/vscode/devcontainers/go:0-${VARIANT}

# NODE_VERSION defined in devcontainer.json
ARG NODE_VERSION="none"
RUN if [ "${NODE_VERSION}" != "none" ]; then su vscode -c "umask 0002 && . /usr/local/share/nvm/nvm.sh && nvm install ${NODE_VERSION} 2>&1"; fi

# [Optional] Uncomment this section to install additional OS packages.
# RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
#     && apt-get -y install --no-install-recommends <your-package-list-here>

# [Optional] Uncomment the next lines to use go get to install anything else you need
# USER vscode
# RUN go get -x <your-dependency-or-tool>

# [Optional] Uncomment this line to install global node packages.
# RUN su vscode -c "source /usr/local/share/nvm/nvm.sh && npm install -g <your-package-here>" 2>&1