# Save-O-Mat

`docker save` with ease. With a simple HTTP API.

```sh
cat <<EOF > images.txt
alpine
busybox
EOF

./saveomat &
curl -fF "images.txt=@images.txt" localhost:8080/tar > images.tar
# OR
wget 'localhost:8080/tar?image=hello-world&image=busybox' -O images.tar
```
