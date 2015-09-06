'use strict';

function initApp(options) {
	var pathbar = new PathBar({
		path: options.path,
		fs:   options.fs,
	});
	pathbar.on('navigate', function(path) {
		window.location = URLROOT+'/view/'+options.fs+'/'+path;
	});
	$('.fs-header').append(pathbar.$el);

	var files = options.files.sort(function(a, b) {
		return a.path > b.path ? 1
			: a.path < b.path ? -1
			: 0;
	});
	var tileView = new FileTileView({
		files: files,
		fs:    options.fs,
	});
	$('.fs-tilelist-container').append(tileView.$el);

	tileView.on('select', function(file, index, files) {
		if (file.type === 'directory') {
			window.location = URLROOT+'/view/'+options.fs+'/'+file.path;
			return;
		}
	});
}
