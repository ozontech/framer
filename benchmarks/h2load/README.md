How to run
- Run in docker (needs docker-compose v2): `docker compose up --abort-on-container-exit`
- Run local:
    * Install h2load: `sudo apt install nghttp2-client`
    * Run dumb server: `(cd ../dumb-server && make run)`
    * Execute `./run.sh`
