#! /bin/bash

PROMPT=$1

# Leave ~.local/

rm -rf ~/.claude
rm -rf ~/.cache/claude
rm     ~/.claude.json*

CLAUDE_CODE_SIMPLE=y claude \
--allow-dangerously-skip-permissions \
--dangerously-skip-permissions \
--model opus \
--effort high \
--output-format=stream-json \
--verbose \
--print \
"$PROMPT"
