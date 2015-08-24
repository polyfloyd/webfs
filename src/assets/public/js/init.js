'use strict';

function initApp(options) {
	$('.fs-file-list').append(options.files.sort(function(a, b) {
		return a.path > b.path ? 1
			: a.path < b.path ? -1
			: 0;
	}).map(function(file) {
		return (new FileTileView({
			file: file,
			fs:   options.fs,
		})).el;
	}));
}
