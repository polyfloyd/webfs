'use strict';

var PathBar = Backbone.View.extend({
	tagName:   'ul',
	className: 'fs-pathbar',

	initialize: function(args) {
		this.path = args.path;
		this.fs   = args.fs;
		this.render();
	},

	render: function() {
		var self = this;

		var names = this.path.split('/').filter(function(name) {
			return !!name;
		});
		this.$el.html(this.template({
			fs:      this.fs,
			names:   names,
			paths:   names.reduce(function(tupple, name) {
				var path = tupple[1]+'/'+name;
				return [tupple[0].concat([path]), path];
			}, [[], ''])[0],
		}));
		this.$('.pathbar-segment').on('click', function() {
			self.trigger('navigate', $(this).attr('data-path'));
		});
	},

	template: _.template(
		'<li '+
			'class="pathbar-segment pathbar-root" '+
			'data-path="/">'+
			'<%- fs %>'+
		'</li>'+
		'<% names.forEach(function(name, index) { %>'+
			'<li '+
				'class="pathbar-segment" '+
				'data-path="<%- paths[index] %>">'+
				'<%- name %>'+
			'</li>'+
		'<% }) %>'
	),
});
