var navLinks = document.querySelectorAll("nav a");
for (var i = 0; i < navLinks.length; i++) {
	var link = navLinks[i]
	if (link.getAttribute('href') == window.location.pathname) {
		link.classList.add("live");
		break;
	}
}

// Ask for confirmation before submitting forms marked with data-confirm (e.g.
// deleting a snippet). This lives here rather than in an inline onsubmit
// attribute because our Content-Security-Policy blocks inline scripts.
var confirmForms = document.querySelectorAll("form[data-confirm]");
for (var j = 0; j < confirmForms.length; j++) {
	confirmForms[j].addEventListener("submit", function (event) {
		if (!window.confirm(this.getAttribute("data-confirm"))) {
			event.preventDefault();
		}
	});
}
