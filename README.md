# Snippetbox
Snippetbox is a simple pastebin application written in Go. It allows users to create and share code snippets with others.

## Routing requests
Route pattern   | Hanfler   | Action
------------- | --------- | ------
`GET /`             | home      | Display the home page
`GET /snippet/view` | snippetView    | Display a specific snippet
`GET /snippet/create`   | snippetCreate      | Display a form for creating a new snippet
`POST /snippet/create`   | snippetCreatePost      | Save a new snippet
`GET /static/`   | http.FileServer      | Serve a specific static file