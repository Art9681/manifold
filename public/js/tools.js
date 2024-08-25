document.addEventListener('DOMContentLoaded', function () {
    const toolView = document.getElementById('tools-container');
    const modelView = document.getElementById('info');
    let settingsVisible = true;

    window.toggleSettings = function () {
        settingsVisible = !settingsVisible;

        if (settingsVisible) {
            showElement(toolView);
            showElement(modelView);
        } else {
            hideElement(toolView);
            hideElement(modelView);
        }

        // Trigger a resize event to make sure the layout adjusts
        window.dispatchEvent(new Event('resize'));
    };

    function showElement(element) {
        element.style.display = 'block';
        setTimeout(() => {
            element.classList.remove('hide');
            element.classList.add('show');
        }, 10);
    }

    function hideElement(element) {
        element.classList.remove('show');
        element.classList.add('hide');
        setTimeout(() => {
            element.style.display = 'none';
        }, 300);
    }
});