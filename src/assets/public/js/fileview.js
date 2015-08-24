'use strict';

var FileView = Poly.View.extend();

FileView.prototype.init = function(file, tmpl) {
	this.hasThumb = file.hasThumb;
	this.name     = file.name;
	this.path     = file.path;
	this.type     = file.type;
	this.el = $(Mustache.render(tmpl, this))[0];
};
