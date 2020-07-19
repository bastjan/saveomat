# Save-O-Mat

[![gh actions](https://github.com/bastjan/saveomat/workflows/Go/badge.svg)](https://github.com/bastjan/saveomat/actions?query=workflow%3AGo) [![codecov](https://codecov.io/gh/bastjan/saveomat/branch/master/graph/badge.svg)](https://codecov.io/gh/bastjan/saveomat) [![Docker Pulls](https://img.shields.io/docker/pulls/bastjan/saveomat)](https://hub.docker.com/r/bastjan/saveomat)

`docker save` with ease. With a simple HTTP API.

```sh
docker run -v /var/run/docker.sock:/var/run/docker.sock -p 8080:8080 bastjan/saveomat

cat <<EOF > images.txt
alpine
busybox
EOF
curl -fF "images.txt=@images.txt" localhost:8080/tar > images.tar
# OR
wget 'localhost:8080/tar?image=hello-world&image=busybox' -O images.tar
```

## FAQ

### Hosting Under a Subpath

The `BASE_URL` environment variable allows hosting under a subpath.

If the value of `BASE_URL` is `/saveomat` the image request becomes `localhost:8080/saveomat/tar`.

### Authentication

To pull private repositories or images an optional `config.json` can be provided.
The file should be in the docker client config format and can usually be found under `$HOME/.docker/config.json`.

⚠️ While private images are not accessible without authentication, they are cached on the server.

Authentication only works for POST requests.

```sh
curl -fF "images.txt=@images.txt" -F "config.json=@$HOME/.docker/config.json" http://localhost:8080/tar > images.tar
```
