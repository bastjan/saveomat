package server

const indexHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>Save-O-Mat</title>
</head>
<body>
<h2>Upload images.txt file</h2>

<form action="./tar" method="post" enctype="multipart/form-data">
    Images file: <input type="file" name="images.txt"><br><br>
    <input type="submit" value="Submit">
</form>
</body>
</html>
`
