# Install Xcode

If your Mac OS is running Mojave, you will need to install Xcode.

* Install Xcode from this [website](https://developer.apple.com/xcode/).

* Accept the Terms and Conditions.

* Ensure that the Xcode app is in the `/Applications` directory. Do not put it
  in `/Users/{user}/Applications)`.

* Point ``xcode-select`` to the Xcode app Developer directory using the
  following command:

```
  sudo xcode-select -s /Applications/Xcode.app/Contents/Developer
```

**Note**: Make sure your Xcode application path is correct.
   * Xcode: `/Applications/Xcode.app/Contents/Developer`
   * Xcode-beta: `/Applications/Xcode-beta.app/Contents/Developer`

