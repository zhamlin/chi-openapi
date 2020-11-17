{ buildGoModule, fetchFromGitHub, stdenv }:
buildGoModule rec {
  name = "enumer-dev-custom";
  pname = "enumer";
  commit = "203dc323c24a7699eeeaaad8bebd9cbe705ce2c0";

  src = fetchFromGitHub {
    owner = "zhamlin";
    repo = "enumer";
    rev = "${commit}";
    sha256 = "02n65xzcvz0ifrrzqzdjlm4445yvmvxgzhkiwy6dn480apjcy6a4";
  };

  vendorSha256 = "02212v028b6snaii3m4088z9bkkz3j5shgqpv47hwazj8z9sl4p1";
  subPackages = [ "." ];
}
