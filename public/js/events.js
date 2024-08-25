// Dynamically adjusts the height of a textarea based on its content.
// Ensures the textarea doesn't exceed a maximum number of rows.
// The textarea's height is updated whenever its content changes.
const textarea = document.getElementById('message');
const maxRows = 8;
textarea.addEventListener('input', function () {
    this.style.height = 'auto';
    this.style.height = `${Math.min(this.scrollHeight, this.clientHeight * maxRows)}px`;
});

// Shift key modifier for text area
document.getElementById('message').addEventListener('keydown', function (event) {
    if (event.key === 'Enter') {
        if (!event.shiftKey) {
            event.preventDefault(); // Prevent the default behavior of the Enter key.

            // Trigger the HTMX request manually.
            htmx.trigger('#send', 'click');

            // Clear the textarea and reset its height
            this.value = '';
            this.style.height = 'auto';
            this.style.height = `${Math.min(this.scrollHeight, this.clientHeight * 1)}px`;
        } else {
            // Shift+Enter: Insert a new line
            // The default behavior will insert a new line, so we don't need to do anything here.
        }
    }
});

// Scroll events on the chat view element. When the user scrolls up, it shows a “Scroll to Bottom” button.
// When the button is clicked or the user scrolls back down, it scrolls the chat view to the bottom and hides the button.
var userHasScrolled = false;

document.getElementById("chat-view").addEventListener("scroll", function () {
    const chatView = document.getElementById("chat-view");
    const scrollToBottomBtn = document.getElementById("scroll-to-bottom-btn");

    userHasScrolled = chatView.scrollHeight - chatView.scrollTop > chatView.clientHeight + 1;

    if (userHasScrolled) {
        scrollToBottomBtn.style.display = 'block';
    } else {
        scrollToBottomBtn.style.display = 'none';
    }
});

document.getElementById("scroll-to-bottom-btn").addEventListener("click", function () {
    const chatView = document.getElementById("chat-view");
    chatView.scrollTop = chatView.scrollHeight;
    userHasScrolled = false;
    this.style.display = 'none';
});
