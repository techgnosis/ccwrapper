#! /usr/bin/env bash

podman run \
-it \
--rm \
--userns=keep-id \
-v ~/.claude.json:/home/claude/.claude.json \
-v ~/.claude:/home/claude/.claude \
-v ~/.gitconfig:/home/claude/.gitconfig \
-v ~/.ssh:/home/claude/.ssh:ro \
-v $(pwd):/agentbox \
--workdir /agentbox \
-e CLAUDE_CODE_OAUTH_TOKEN \
agentbox:1 bash
