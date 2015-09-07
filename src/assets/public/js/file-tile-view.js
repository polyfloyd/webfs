'use strict';

var FileTileView = Backbone.View.extend({
	initialize: function(args) {
		var self = this;

		this.files = args.files;
		this.setElement(this.template({
			files:   this.files,
			icons:   this.icons,
			fs:      args.fs,
			urlroot: URLROOT,
		}));

		this.$('li').on('click', function(event) {
			event.preventDefault();
			var index = parseInt($(this).attr('data-index'), 10);
			self.trigger('select', self.files[index], index, self.files);
		});
	},

	icons: {
		directory: 'fa fa-folder',
		image:     'fa fa-picture-o',
		video:     'fa fa-video-camera',
	},

	template: _.template(
		'<ul class="file-tilelist">'+
			'<% files.forEach(function(file, index) { %>'+
				'<li '+
					'class="file-tile file-type-<%- file.type %> <%= file.hasThumb ? \'fs-thumb\' : \'\' %>" '+
					'data-index="<%= index %>">'+
					'<div class="tile-icon <%= icons[file.type] %>"></div>'+
					'<div '+
						'class="tile-background"'+
						'title="<%- name %>"'+
						'style="<% if (file.hasThumb) { %>'+
							'background-image: url(\'<%= urlroot %>/thumb/<%= fs %>/<%- file.path.replace(/\'/g, \'\\\\\\\'\') %>.jpg\')'+
						'<% } %>">'+
							'<p class="file-title"><%- file.name %></p>'+
						'</div>'+
				'</li>'+
			'<% }) %>'+
		'</ul>'
	),
});
