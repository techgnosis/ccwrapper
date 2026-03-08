FROM alpine:3.23

RUN apk update && apk add curl bash git openssh go make

RUN adduser -D -u 501 -g 20 claude

USER claude
WORKDIR /home/claude/


ENV PATH="/home/claude/.local/bin:${PATH}"


RUN curl -fsSL https://claude.ai/install.sh | bash
RUN curl -fsSL https://raw.githubusercontent.com/Dicklesworthstone/beads_rust/main/install.sh | bash -s -- --skip-skills --version v0.1.14
