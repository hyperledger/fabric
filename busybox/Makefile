BUSYBOX_VER=1.25.1
BUSYBOX_URL=https://www.busybox.net/downloads/busybox-$(BUSYBOX_VER).tar.bz2

OBJDIR=build/busybox-$(BUSYBOX_VER)

all: $(OBJDIR)/busybox

install: $(BINDIR)/busybox

$(BINDIR)/busybox: $(OBJDIR)/busybox
	mkdir -p $(@D)
	cp $< $@

$(OBJDIR)/.source:
	mkdir -p $(@D)
	curl -L $(BUSYBOX_URL) | (cd $(@D); tar --strip-components=1 -jx)
	touch $@

$(OBJDIR)/.config: $(OBJDIR)/.source
	make -C $(@D) defconfig

$(OBJDIR)/busybox: Makefile $(OBJDIR)/.config
	make -C $(@D) -l 2.5 -j all LDFLAGS=-static

clean:
	-rm -rf $(OBJDIR)

