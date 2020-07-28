package server

const indexHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>Save-O-Mat</title>
    <style>
        body {
            font-family: sans-serif;
            max-width: 960px;
            margin: auto;
        }
        code, pre.codeblock {
            background-color: #eceff1;
            padding: 3px;
            border-radius: 3px;
            font-family: monospace;
            overflow-y: auto;
        }
        form {
            margin-top: 12px;
        }
        .ext-url, .dark-red {
            color: #BF360C;
        }
        .dark-blue {
            color: #1a237e;
        }
    </style>
</head>
<body>

<!--
Modern GitHub Corners
Copyright (c) 2016 Tim Holman - http://tholman.com
The MIT License
-->
<a href="https://github.com/bastjan/saveomat" class="github-corner" aria-label="View source on GitHub"><svg width="80" height="80" viewBox="0 0 250 250" style="fill:#FD6C6C; color:#fff; position: absolute; top: 0; border: 0; right: 0;" aria-hidden="true"><path d="M0,0 L115,115 L130,115 L142,142 L250,250 L250,0 Z"></path><path d="M128.3,109.0 C113.8,99.7 119.0,89.6 119.0,89.6 C122.0,82.7 120.5,78.6 120.5,78.6 C119.2,72.0 123.4,76.3 123.4,76.3 C127.3,80.9 125.5,87.3 125.5,87.3 C122.9,97.6 130.6,101.9 134.4,103.2" fill="currentColor" style="transform-origin: 130px 106px;" class="octo-arm"></path><path d="M115.0,115.0 C114.9,115.1 118.7,116.5 119.8,115.4 L133.7,101.6 C136.9,99.2 139.9,98.4 142.2,98.6 C133.8,88.0 127.5,74.4 143.8,58.0 C148.5,53.4 154.0,51.2 159.7,51.0 C160.3,49.4 163.2,43.6 171.4,40.1 C171.4,40.1 176.1,42.5 178.8,56.2 C183.1,58.6 187.2,61.8 190.9,65.4 C194.5,69.0 197.7,73.2 200.1,77.6 C213.8,80.2 216.3,84.9 216.3,84.9 C212.7,93.1 206.9,96.0 205.4,96.6 C205.1,102.4 203.0,107.8 198.3,112.5 C181.9,128.9 168.3,122.5 157.7,114.1 C157.9,116.9 156.7,120.9 152.7,124.9 L141.0,136.5 C139.8,137.7 141.6,141.9 141.8,141.8 Z" fill="currentColor" class="octo-body"></path></svg></a><style>.github-corner:hover .octo-arm{animation:octocat-wave 560ms ease-in-out}@keyframes octocat-wave{0%,100%{transform:rotate(0)}20%,60%{transform:rotate(-25deg)}40%,80%{transform:rotate(10deg)}}@media (max-width:500px){.github-corner:hover .octo-arm{animation:none}.github-corner .octo-arm{animation:octocat-wave 560ms ease-in-out}}</style>

<h2>Bundle Docker Images</h2>

<p>
    Save-O-Mat bundles docker images like <code>docker save</code> does.
</p>

<p>
    Upload a <code>images.txt</code> file; Save-O-Mat will pull all
    required images and save them to a tar archive.
</p>

<h3>Download archive</h3>

<form action="tar" method="post" enctype="multipart/form-data">
    <label>Images file: <input type="file" name="images.txt"></label><br><br>
    <label>Optional auth (<code>~/.docker/config.json</code>): <input type="file" name="config.json"></label><br><br>
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

<h3>Authentication</h3>

<p>
    To pull private repositories or images an optional <code>config.json</code> can be provided.
    The file should be in the docker client config format and can usually be found under <code>$HOME/.docker/config.json</code>.
</p>

<p>
    ⚠️ While private images are not accessible without authentication, they are cached on the server.
</p>

<p>
    Authentication only works for POST requests.
</p>

<pre class="codeblock">
curl -fF "images.txt=@images.txt" <span class="dark-blue">-F "config.json=@$HOME/.docker/config.json"</span> <span class="ext-url">EXTERNAL_URL/</span> > images.tar
</pre>

<h3>Helm</h3>
Save-O-Mat can find docker images used with a helm chart. Charts are rendered using a user provided <code>values.yaml</code> file, producing a kubernetes manifest which is then scanned for container images.

Simply <code>POST</code> a chart reference along with your <code>values.yaml</code> file to receive the image tarball. The chart repository is handled transparently. The following options are currently available via POST parameters:
<br />
<br />
<table>
  <tr>
    <th align="left">Parameter</th>
    <th align="left">Description</th>
  </tr>
  <tr>
    <td>repoName</td>
    <td>Name of the chart repository hosting the chart</td>
  </tr>
  <tr>
    <td>repoURL</td>
    <td>URL of the chart repository</td>
  </tr>
  <tr>
    <td>username</td>
    <td>Username for the chart repo (optional)</td>
  </tr>
  <tr>
    <td>password</td>
    <td>Password for the chart repo (optional)</td>
  </tr>
  <tr>
    <td>chart</td>
    <td>The chart reference</td>
  </tr>
  <tr>
    <td>version</td>
    <td>The chart version (optional)</td>
  </tr>
  <tr>
    <td>values.yaml</td>
    <td>Custom values.yaml file that will be applied during rendering</td>
  </tr>
  <tr>
    <td>verify</td>
    <td>Enable chart verification (optional, set to any value to enable)</td>
  </tr>
  <tr>
    <td>auth</td>
    <td>config.json file for docker daemon to use (optional, same as in above Authentication section)</td>
  </tr>
</table><br />

Example with the <code>hackmd</code> chart:
<pre class="codeblock">
curl -fF "values.yaml=@values.yaml" -F "chart=stable/hackmd" -F "repoName=stable" -F "repoURL=https://kubernetes-charts.storage.googleapis.com" http://localhost:8080/helm -o test.tar
</pre>

The example uses the following values.yaml file:
<pre class="codeblock">
image:
  repository: hackmdio/hackmd
  tag: 1.0.1-ce-alpine
  pullPolicy: IfNotPresent
postgresql:
  install: true
  image:
    tag: "9.6"
  postgresUser: "hackmd"
  postgresDatabase: "hackmd"
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
