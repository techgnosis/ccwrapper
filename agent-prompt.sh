#! /bin/bash

PROMPT=$1

CLAUDE_CODE_SIMPLE=y claude \
--allow-dangerously-skip-permissions \
--dangerously-skip-permissions \
--model opus \
--effort high \
--output-format=stream-json \
--verbose \
--print \
-e CLAUDE_CODE_OAUTH_TOKEN \
"$PROMPT"
