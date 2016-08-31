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
					if (typeof icon.match === 'function') {
						return icon.match(file);
					}
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
			match: function(file) {
				return file.type.match(/^directory$/) && !file.isUnlocked;
			},
			class: 'fa-lock tile-icon-show',
		},
		{
			match: [ /^directory$/ ],
			class: 'fa-folder',
		},
		{
			match: [ /^video/, /^image\/gif$/ ],
			class: 'fa-play tile-icon-show',
		},
		{
			match: [ /^image/ ],
			class: '', // Don't show an icon for images.
		},
		{
			match: [ /^text/, /^application\/pdf$/ ],
			class: 'fa-file-text',
		},
		{
			match: [ /^.*$/ ],
			class: 'fa-file',
		},
	],

	template: _.template(
		'<ul class="file-tilelist">'+
			'<% files.forEach(function(file, index) { %>'+
				'<li '+
					'class="file-tile file-type-<%- file.type.replace(/\\W/g, \'-\') %> <%= file.hasThumb ? \'fs-thumb\' : \'\' %>" '+
					'data-index="<%= index %>">'+
					'<div class="tile-icon fa fa-fw fa-5x <%= iconClass(file) %>"></div>'+
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
