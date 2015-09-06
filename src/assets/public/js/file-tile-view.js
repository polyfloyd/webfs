'use strict';

var FileTileView = Backbone.View.extend({
	initialize: function(args) {
		var self = this;

		this.files = args.files;
		this.setElement(this.template({
			files:   this.files,
			fs:      args.fs,
			urlroot: URLROOT,
		}));

		this.$('li').on('click', function(event) {
			event.preventDefault();
			var index = parseInt($(this).attr('data-index'), 10);
			self.trigger('select', self.files[index], index, self.files);
		});
	},

	template: _.template(
		'<ul class="file-tilelist">'+
			'<% files.forEach(function(file, index) { %>'+
				'<li '+
					'class="file-tile file-type-<%- file.type %> col-xs-1"'+
					'data-index="<%= index %>">'+
					'<div '+
						'class="tile-background <%= file.hasThumb ? \'fs-thumb\' : \'\' %>"'+
						'title="<%- name %>"'+
						'style="<% if (file.hasThumb) { %>'+
							'background-image: url(\'<%= urlroot %>/thumb/<%= fs %>/<%- file.path.replace(/\'/g, \'\\\\\\\'\') %>.jpg\')'+
						'<% } %>"></div>'+
					'<p class="file-title"><%- file.name %></p>'+
				'</li>'+
			'<% }) %>'+
		'</ul>'
	),
});
