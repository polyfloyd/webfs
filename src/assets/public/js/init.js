'use strict';

function initApp(options) {
	$('.fs-file-list').append(options.files.map(function(file) {
		return (new FileTileView({
			file: file,
			fs:   options.fs,
		})).el;
	}));
}
