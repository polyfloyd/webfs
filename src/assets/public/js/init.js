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
		function dirs(a, b) {
			return a.type === 'directory' && b.type !== 'directory' ? -1
				: b.type === 'directory' && a.type !== 'directory' ? 1
				: 0;
		}
		function paths(a, b) {
			return a.path > b.path ? 1
				: a.path < b.path ? -1
				: 0;
		}
		return dirs(a, b) || paths(a, b);
	});
	var tileView = new FileTileView({
		files: files,
		fs:    options.fs,
	});
	$('.fs-tilelist-container').append(tileView.$el);

	tileView.on('select', function(file, index, files, $el) {
		if (file.type === 'directory') {
			window.location = URLROOT+'/view/'+options.fs+'/'+file.path;
			return;
		}
		var embed = new FileEmbedView({
			fs:    options.fs,
			files: files,
			index: index,
		});
		embed.popup($el);
	});
}
