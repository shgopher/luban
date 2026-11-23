FROM alpine:3.20

RUN apk add --no-cache tmux git bash

WORKDIR /work
COPY agent.sh run.sh verify.sh ./
RUN chmod +x agent.sh run.sh verify.sh

CMD ["bash", "-c", "bash /work/run.sh && bash /work/verify.sh"]
