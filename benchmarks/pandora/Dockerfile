FROM ubuntu:24.04
WORKDIR /app
ADD https://github.com/yandex/pandora/releases/download/v0.5.26/pandora_0.5.26_linux_amd64 /usr/local/bin/pandora
RUN chmod +x /usr/local/bin/pandora
ENTRYPOINT ["pandora"]
