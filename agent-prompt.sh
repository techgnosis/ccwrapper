#! /bin/bash

PROMPT=$1

./clean-claude.sh

CLAUDE_CODE_SIMPLE=y claude \
--allow-dangerously-skip-permissions \
--dangerously-skip-permissions \
--model opus \
--effort high \
--output-format=stream-json \
--verbose \
--print \
"$PROMPT" | tee example.json
