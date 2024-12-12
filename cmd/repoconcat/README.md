# Concatenates text files into one file

```
$ go run main.go -paths="/path1,/path2" -types=".txt,.go" -ignore="_test" -output="result.txt" -recursive=true
```

go run main.go -paths="/Users/arturoaquino/Documents/manifold" -types=".go" -ignore="_test" -output="manifold.txt" -recursive=false

ctags -R --fields=+n --output-format=json -o tags.json /Users/arturoaquino/Documents/manifold

go run main.go -paths="/Users/arturoaquino/Documents/manifold" -types=".go" -ignore="_test" -output="manifold_documents.txt" -recursive=false