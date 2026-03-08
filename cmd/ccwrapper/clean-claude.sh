#! /usr/bin/env bash

echo "Deleting most claude state"

if [ -f ~/.claude/credentials.json ]; then
    echo "Backing up credentials.json"
    cp ~/.claude/credentials.json /tmp/claude-credentials.json
    rm -rf ~/.claude
    mkdir -p ~/.claude
    mv /tmp/claude-credentials.json ~/.claude/credentials.json
else
    rm -rf ~/.claude
fi

rm -rf ~/.cache/claude
