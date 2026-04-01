# Snippetbox
Snippetbox is a simple pastebin application written in Go. It allows users to create and share code snippets with others.

## Routing requests
Route pattern   | Hanfler   | Action
------------- | --------- | ------
`/`             | home      | Display the home page
`/snippet/view` | snippetView    | Display a specific snippet
`/snippet/create`   | snippetCreate      | Display a form for creating a new snippet