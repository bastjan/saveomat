# Save-O-Mat

`docker save` with ease. Over the network.

```sh
cat <<EOF > images.txt
alpine
busybox
EOF

./saveomat &
curl -fF "images.txt=@images.txt" localhost:8080/tar > images.tar
```
