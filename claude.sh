#! /bin/bash

./cmd/ccwrapper/clean-claude.sh

claude \
--allow-dangerously-skip-permissions \
--dangerously-skip-permissions \
--model opus \
--effort high
