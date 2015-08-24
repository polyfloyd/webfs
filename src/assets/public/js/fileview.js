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
			'class="fs-file col-md-1"'+
			'href="<%= urlroot %>/fs/<%= fs %>/view<%- file.path %>">'+
			'<div '+
				'class="fs-file-background fs-file-type-<%- file.type %> <%= file.hasThumb ? \'fs-url\' : \'\' %>"'+
				'title="<%- name %>"'+
				'style="<% if (file.hasThumb) { %>background-image: url(\'<%= urlroot %>/fs/<%= fs %>/thumb<%- file.path %>\')<% } %>">'+
			'</div>'+
			'<p class="fs-file-title"><%- file.name %></p>'+
		'</a>'
	),
});
