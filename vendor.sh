mv vendor ../vendor
mv vendor.conf ../vendor.conf 
mkdir vendor 
touch vendor.conf 
echo "github.com/rancher/types $1 file:///Users/kshah/go/src/github.com/rancher/types/.git" >> vendor.conf
source ~/.go_profile
trash
rm -rf ../vendor/github.com/rancher/types
mv vendor/github.com/rancher/types ../vendor/github.com/rancher/types
rm -rf vendor 
rm -rf vendor.conf
mv ../vendor ./vendor
mv ../vendor.conf ./vendor.conf
