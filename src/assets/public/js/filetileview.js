'use strict';

var FileTileView = Backbone.View.extend({
	initialize: function(args) {
		this.file = args.file;
		this.fs   = args.fs;
		this.setElement($(this.template({
			file:    this.file,
			fs:      args.fs,
			urlroot: URLROOT,
		}))[0]);
	},

	template: _.template(
		'<a '+
			'class="fs-file fs-file-type-<%- file.type %> col-md-1"'+
			'href="<%= urlroot %>/view/<%= fs %>/<%- file.path %>">'+
			'<div '+
				'class="fs-file-background <%= file.hasThumb ? \'fs-thumb\' : \'\' %>"'+
				'title="<%- name %>"'+
				'style="<% if (file.hasThumb) { %>'+
					'background-image: url(\'<%= urlroot %>/thumb/<%= fs %>/<%- file.path.replace(/\'/g, \'\\\\\\\'\') %>.jpg\')'+
				'<% } %>">'+
			'</div>'+
			'<p class="fs-file-title"><%- file.name %></p>'+
		'</a>'
	),
});
