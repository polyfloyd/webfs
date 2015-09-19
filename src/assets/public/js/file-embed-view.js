'use strict';

var FileEmbedView = Backbone.View.extend({
	initialize: function(args) {
		this.files = args.files;
		this.fs    = args.fs;
		this.index = args.index || 0;
		this.render();
	},

	render: function() {
		var self = this;

		this.setElement($(this.template()));
		this.$('.do-prev').on('click', function() {
			self.seek(-1);
		});
		this.$('.do-next').on('click', function() {
			self.seek(1);
		});
	},

	renderCurrentFile: function() {
		var self = this;

		var file = this.files[this.index];
		var view = this.fileViewTemplates[file.type];
		if (!view) {
			view = function() { return ''; };
		}

		this.$('.embed-container').html(this.contentTemplate({
			file:     file,
			fileView: view({
				urlroot: URLROOT,
				file:    file,
				fs:      this.fs,
			}),
		}));
		this.$('.embed-content .embed-media').on('load canplay', function() {
			self.resizeContent();
		});
		this.$('.embed-content .embed-close').on('click', function() {
			self.close();
		});
	},

	resizeContent: function() {
		console.log('resize')
		var $content = this.$('.embed-content');
		$content.css({
			width:  null,
			height: null,
		});

		// ensure the element is displayed before querying its dimensions.
		_.defer(function(self) {
			var $container  = self.$('.embed-container');
			var $media      = self.$('.embed-media');
			var viewport    = { x: $container.width(), y: $container.height() };
			var contentSize = { x: $media.width(),     y: $media.height() };

			var ratio = 1;
			if (viewport.x < contentSize.x) {
				ratio = viewport.x / contentSize.x;
			}
			if (viewport.y < contentSize.y * ratio) {
				ratio = viewport.y / contentSize.y;
			}

			$content.css({
				width:  (ratio * contentSize.x)+'px',
				height: (ratio * contentSize.y)+'px',
			});
			$content.removeClass('loading');
		}, this);
	},

	popup: function($expandFrom) {
		var self = this;
		$('body > .file-embed').remove();
		$('body').prepend(this.$el);
		this.renderCurrentFile();
	},

	close: function() {
		this.$el.remove();
	},

	seek: function(delta) {
		var oldIndex = this.index;
		this.index += delta;
		if (this.index < 0) {
			this.index = 0;
		} else if (this.index >= this.files.length) {
			this.index = this.files.length - 1;
		}
		if (oldIndex !== this.index) {
			this.trigger('seek', this.index);
			this.renderCurrentFile();
		}
	},

	template: _.template(
		'<div class="file-embed">'+
			'<div class="embed-bg"></div>'+

			'<a class="embed-seek do-prev fa fa-chevron-left"></a>'+
			'<div class="embed-container"></div>'+
			'<a class="embed-seek do-next fa fa-chevron-right"></a>'+
		'</div>'
	),
	contentTemplate: _.template(
		'<div class="embed-content loading file-type-<%- file.type %>">'+
			'<a class="embed-close fa fa-close" title="Close"></a>'+
			'<%= fileView %>'+
			'<p class="embed-title"><%- file.name %></p>'+
		'</div>'
	),
	fileViewTemplates: {
		'image': _.template(
			'<img class="embed-media" src="<%= urlroot %>/view/<%= fs %>/<%- file.path %>" />'
		),
		'video': _.template(
			'<video class="embed-media" controls autoplay loop>'+
				'<source type="video/mp4" src="<%= urlroot %>/view/<%= fs %>/<%- file.path %>?fmt=video%2Fmp4" />'+
				'<source type="video/webm" src="<%= urlroot %>/view/<%= fs %>/<%- file.path %>?fmt=video%2Fwebm" />'+
			'</video>'
		),
	},
});