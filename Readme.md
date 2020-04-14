# Save-O-Mat

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
