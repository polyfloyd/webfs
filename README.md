webfs
=====

A daemon that presents the contents of one or more directories through a
web-interface.


## Configuring
Copy and modify `config.example.json` accordingly. By default,
`$PWD/config.json` is used. This location can be overridden with the `-conf`
option.

Some thumbnail processors require an external program to function:
* Vector images (e.g. svg and pdf) require Inkscape
* Videos require ffmpeg

### Special Files
It's important to note that dotfiles (filenames starting with `.`) are hidden.

### .passwd.txt
Create a file called `.passwd.txt` in a directory that you would like to
protect. All underlying files will require a username and password to be
accessible.

The format of the file requires a username and password separated by a single
space. It is possible to specify multiple username/password pairs on separate
lines.

Example:
```
tarzan correcthorsebatterystaple
jane mysecretpassword
```

### .icon.(png|jpe?g)
By default, the thumbnail of a directory will be based on its contents. If
you'd like to set a custom thumbnail, name an image file accordingly.


## Screenshots
![directory overview](media/example-directory.png)
![view an image](media/example-image.png)
