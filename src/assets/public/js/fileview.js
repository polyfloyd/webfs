'use strict';

var FileView = Poly.View.extend();

FileView.prototype.init = function(file, tmpl) {
  tmpl = tmpl.replace(/{%\s*\.name\s*%}/g, file.name); // TODO: use a real template engine
  tmpl = tmpl.replace(/{%\s*\.path\s*%}/g, file.path);
  this.el = $(tmpl)[0];
};
