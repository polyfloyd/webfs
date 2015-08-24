'use strict';

function trackEvent(key, value) {
	if (window._paq) {
		_paq.push(['trackEvent', key, value]);
	}
}

function initApp(options) {
	var fileList         = $('.fs-file-list');
	var fileListTemplate = $('#fs-file-tmpl').html();

	Mustache.parse(fileListTemplate);

	fileList.append(options.files.map(function(file) {
		return (new FileView(file, fileListTemplate)).el;
	}));
}
