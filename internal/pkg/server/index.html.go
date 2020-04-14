package server

const indexHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>Save-O-Mat</title>
    <style>
        body {
            font-family: sans-serif;
            max-width: fit-content;
            max-width: -moz-fit-content;
            margin: auto;
        }
        code, pre.codeblock {
            background-color: #eceff1;
            padding: 3px;
            border-radius: 3px;
            font-family: monospace;
        }
        form {
            margin-top: 12px;
        }
        .ext-url {
            color: #BF360C;
        }
    </style>
</head>
<body>
<h2>Bundle Docker Images</h2>

<p>
    Save-O-Mat bundles docker images like <code>docker save</code> does.
</p>

<p>
    Upload a <code>images.txt</code> file; Save-O-Mat will pull all
    required images and save them to a tar archive.
</p>

<h3>Download Archive</h3>

<form action="tar" method="post" enctype="multipart/form-data">
    <label>Images File: <input type="file" name="images.txt"></label><br><br>
    <input type="submit" value="Download archive">
</form>

<h3>File format</h3>

<pre class="codeblock"># lines with # in the beginning are ignored
busybox
# empty lines are ignored too

# list as many images as you like...
golang:alpine
debian:buster
</pre>

<h3>Use curl or wget</h3>

<pre class="codeblock">
curl -fF "images.txt=@images.txt" <span class="ext-url">EXTERNAL_URL/</span> > images.tar
# OR
wget '<span class="ext-url">EXTERNAL_URL/</span>?image=hello-world&image=busybox' -O images.tar
</pre>

<script>
    var extUrl = new URL("tar", window.location.href).href;
    var x = document.getElementsByClassName("ext-url");
    var i;
    for (i = 0; i < x.length; i++) {
        x[i].innerText = extUrl;
    }
</script>
</body>
</html>
`
