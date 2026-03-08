#! /bin/bash

PROMPT=$1

claude \
--allow-dangerously-skip-permissions \
--dangerously-skip-permissions \
--model opus \
--effort high
