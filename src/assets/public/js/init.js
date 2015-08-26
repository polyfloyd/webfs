'use strict';

function initApp(options) {
	var pathbar = new PathBar({
		path: options.path,
		fs:   options.fs,
	});
	pathbar.on('navigate', function(path) {
		window.location = URLROOT+'/fs/'+options.fs+'/view'+path;
	});
	$('.fs-header').append(pathbar.$el);

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
