'use strict';

var FileTileView = Backbone.View.extend({
	initialize: function(args) {
		var self = this;

		this.files = args.files;
		this.setElement(this.template({
			files:     this.files,
			fs:        args.fs,
			urlroot:   URLROOT,
			iconClass: function(file) {
				return self.icons.find(function(icon) {
					return icon.match.some(function(expression) {
						return file.type.match(expression);
					});
				}).class;
			},
		}));

		this.$('li').on('click', function(event) {
			event.preventDefault();
			var $self = $(this);
			var index = parseInt($self.attr('data-index'), 10);
			self.trigger('select', self.files[index], index, self.files, $self);
		});
	},

	icons: [
		{
			match: [ /^directory$/ ],
			class: 'fa fa-folder',
		},
		{
			match: [ /^video/, /^image\/gif$/ ],
			class: 'fa fa-video-camera',
		},
		{
			match: [ /^image/ ],
			class: 'fa fa-picture-o',
		},
		{
			match: [ /^text/, /^application\/pdf$/ ],
			class: 'fa fa-file-text',
		},
		{
			match: [ /^.*$/ ],
			class: 'fa fa-file',
		},
	],

	template: _.template(
		'<ul class="file-tilelist">'+
			'<% files.forEach(function(file, index) { %>'+
				'<li '+
					'class="file-tile file-type-<%- file.type.replace(/\\W/g, \'-\') %> <%= file.hasThumb ? \'fs-thumb\' : \'\' %>" '+
					'data-index="<%= index %>">'+
					'<div class="tile-icon <%= iconClass(file) %>"></div>'+
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
