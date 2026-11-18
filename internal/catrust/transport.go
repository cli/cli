// Package catrust applies additional CA distrust to HTTPS connections.
package catrust

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
)

// NewTransport clones base and rejects verified chains containing a distrusted
// eMudhra CA key. At least one allowed chain must remain, as in Go's verifier.
func NewTransport(base http.RoundTripper) (http.RoundTripper, error) {
	transport, ok := base.(*http.Transport)
	if !ok || transport == nil {
		return nil, fmt.Errorf("CA distrust requires a non-nil *http.Transport, got %T", base)
	}
	if transport.DialTLSContext != nil || transport.DialTLS != nil {
		return nil, errors.New("CA distrust does not support custom TLS dialers")
	}
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		return nil, errors.New("CA distrust requires standard certificate verification")
	}
	// Peer callbacks run before our chain filtering.
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.VerifyPeerCertificate != nil {
		return nil, errors.New("CA distrust does not support VerifyPeerCertificate callbacks")
	}

	cloned := transport.Clone()
	if cloned.TLSClientConfig == nil {
		cloned.TLSClientConfig = &tls.Config{}
	}
	previous := cloned.TLSClientConfig.VerifyConnection
	cloned.TLSClientConfig.VerifyConnection = func(state tls.ConnectionState) error {
		// Copy before filtering: TLS also retains these chains in resumed sessions.
		state.VerifiedChains = slices.DeleteFunc(slices.Clone(state.VerifiedChains), func(chain []*x509.Certificate) bool {
			for _, cert := range chain {
				hash := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
				if distrustedCAKeys[hex.EncodeToString(hash[:])] {
					return true
				}
			}
			return len(chain) == 0
		})
		if len(state.VerifiedChains) == 0 {
			return errors.New("x509: certificate chain uses a distrusted CA")
		}
		if previous != nil {
			return previous(state)
		}
		return nil
	}
	return cloned, nil
}

// SHA-256 SPKI hashes of eMudhra-identified CA issuer keys.
// Sources (2026-11-17): https://repository.emsign.com/ and
// https://ccadb.my.salesforce-sites.com/ccadb/AllCertificateRecordsCSVFormatV5
var distrustedCAKeys = map[string]bool{
	"0169613548123dab14afffcb5a3f0e804c70ac903f29ecad55ed43dc75f13881": true,
	"04db3b7ddf683b1d4b5b64fe743a3a8d8c3d71f0c339d7dc25b343e665a9eb5b": true,
	"0a11949b826e7d00a0c5f8d310b2d0d53f77f8402e03742b70a28550094a09da": true,
	"0a6c2b2ca5b364207954171987ac9d2178561569d8205457e77ca90c32ede282": true,
	"0ce39965f7e40aab1da0295834788f192da49a0f216be9248d9bde82b3ad7a95": true,
	"0d7374f51e62e4da75736da7746ac9fcca1eb4e8e0c6575dfb3b8c55665b9c66": true,
	"0f9db7a54896c137102f9d992e4413e49716bbeee02bb834d7e747562542e6b9": true,
	"0fa07b240a5441b610cca2789d747dcdcfe3a396d6e7bda6496b6f91efa90e0a": true,
	"0fd359ea859a7377de6978fb222e4d09f88be511b7b34508574f1e706c4f82d7": true,
	"10999cc87382ee51772de4f6b40e8d11e880ada388780f52d54629b74db191f3": true,
	"11567f2aaf4325c4044880c6ed2b37f1ca2586d87059cf254934d97c4fee89a5": true,
	"120a54edbb14d3cbd2b33ebf67010220374ae6c4f881dea68937bf8bff7174ad": true,
	"12a9538ef01c1f70e01e23134d0657940e390b2af2fd972f99e1284669c49799": true,
	"12dd68cd491523b8e627ab6f192d32d6cbb3655275c17afa61bc511e7ca34d4c": true,
	"15376f44b0b1857bf376a7326727ad61ee0d69e0ffc21868da978c067bf2dffd": true,
	"1a942e3925b809896b4b291a8cd2e7bc5f1d533aa523b8353b2df377b0f0adbe": true,
	"1cbbd4cb5eec6a287e34636c07c78378fa3eb3c6d3fb0563286870f283d87034": true,
	"22fc3bc2fe57a5c91f74594ec63dfb4810f0888d16021929d56386137af606f5": true,
	"2364348969a2ebd69ca678a82358bd2830d3684d7c20feaa2e09efd8e1eaa117": true,
	"23aef4af7046d73d0a5e9acee45fc0eab0208d2f1e81ad31c623cadbd6461ff2": true,
	"27616f251b91b3df6a241b45854a5db0a7b1bedd13a296f5cacb670aeac460e5": true,
	"278aea3b0066f410f0a74b6d341de6e4ef0c805cb4d079d02c79a91a174b7837": true,
	"2ad049000523b8211626e5ae59c141ce7fb320b10207853f6ba7aeb32744663a": true,
	"2ff90b0e2bd10089c13500a8cb86bee9027f7a71f12a1c074aa07bb8a08e2724": true,
	"32159657965e44ec9b1b7f0933475009e50479a0fae1dfbb236a8dad4ff7a6a6": true,
	"33836758bd871b8d464bd669b0b81832880b81e363fb8cbb21ba8e843a138858": true,
	"34556838281688d1c967d43d1372a1bbaf389277222f03e1017b18b348a1f00e": true,
	"376a1a7082a593dccc20d561d119e9ab8d30f11cc321d0a37fa41f0df284e01c": true,
	"38fc3d6e2fb7c591c4c11c5bb1f9f46913be7f1c4acfeb7643f28fea39bea6df": true,
	"3f83a68c4e80fc3f4f487e3105ad146498e56a09a3ea223b2a2982943f5995d7": true,
	"41fdf154707c06d8a9ef871f96c46de0d9fc2405a3d174e0f288f23435561005": true,
	"421de4f594192d7ab488fbd77bf4ffeb71f94d2fab65e0fb77b4cc3d80769c34": true,
	"42eddd4b85ac2c08029105b677f0aca89261f77ad3425589be3aa9aa030b0feb": true,
	"43bffa19d6ef651451f6841e019931e96f6f2eaeff28ab4b6f1ce4576de2717b": true,
	"49dd353dc2b919a3770645117d0e60c16115e7d1bc259eefe734b00ea434fe5f": true,
	"4aa7e0e79901d2709e94750e98a013e64dcbdc219c23bdea36b0afaaec4f12d9": true,
	"4cfca6cecda04930a74734fcdddc694c7c5f1a5dad750f6c9e109f5d771e8f2e": true,
	"4e8b3000146768d220e3b7fbc2bd2f46bfb3f76c563f82f0ab8a9b2563eeef00": true,
	"5046252d54982aebf99d11aa0319e02b53fa8e9cb2de8c8b6469c5a5020f4059": true,
	"562f740ca039a1e6e3fe76b9006fbc103d803b1a7f7b3c564edbe24b5e23a50e": true,
	"56ef2b0a6eb61a13e93d422c90010e946fb1fd6244cdce3a36a52b59f7f04d5a": true,
	"56ef977f0f2900d02301d37f103fe78459aa528ef1eae52e738f5a384e33bc46": true,
	"58d28f754898adee937431d74fc1a3bee02b8a663444f254fc63a9a816f75ab8": true,
	"58f70efce7451b29a43b4637eb79fc1afdfeb875d5b2102d8595497f83a54501": true,
	"5bb8639ff2309793b3489b01d68f912171142a136382ab3ef39720b4596210d3": true,
	"5c0660a2b164910d8ff589c18e14552e9e0ec237dcefe2cb736d41a4811f81a0": true,
	"5da77dc800f4c6fa93c6015aa6bd898fdb7c28d869e764571b25c00375a77755": true,
	"60e4ef4e0c3f8bf92e29f1a1327b3de1a63c76f52363054e7a8af142b0256f50": true,
	"628389a5306d2ff30de784a4b4b8abbdcd1272eb0e0a6ecd6d2476f6ed53702a": true,
	"63ecefee431d49523bc1136c4e72d0206a477086caf606cd5cf025686c109cf8": true,
	"6a8df2fca50bf1410515d15a9ff4ac54a08179814bbd1d5a6282fa7cde817101": true,
	"6aa600de06d8e834c2f1835a2766ad685d179b871b02d7bb6ec208680ad25397": true,
	"6b8088e6bcb25f09d74806a06479273b63d5684bc169ed142db0c961415061e5": true,
	"6c9a5a1af4fe131a108b6fda9f51c6dcca39c8cae8b74d8e47217f352e69e49f": true,
	"6ca6b0e819309539b9627e00aa70d74a0919800f8f217e6197fb6946989ed264": true,
	"6e915ef2d8286944381a50006b47d45665642c42803b5aecf7110bb3c9dd5671": true,
	"71f8adeec2b58adbdf6205c98dde928a4254b03695193d7f9a63a9a2588095d0": true,
	"73061185d2e093cd14796559eecaa57697c57718c4cec4a62e3d3caee2a65baf": true,
	"73999087c5a025748f16413ba63caa9d4082dd949242cc2a68d2854917fdd573": true,
	"73f0c5be28dc503cd970ad9c452348306d43a38e956979dc1f5a0bf8b039cf64": true,
	"75caea0c53b22717e992c8f3f423bf2127ab3fac265fb558895f134fc53623c2": true,
	"7ba89c4d89ea3c07aa472895e93d59f9d78f8407e3656f516f3162fbf87ce344": true,
	"7f4d74d75a846857fa5aaab7c322500aef7a3f5a7821ae1f606f55ffb43a3eee": true,
	"8429351035ba90ba14ef3f7ae38ca35480605a0296aa6c263c6a9b26bb9a48e1": true,
	"8813f72085ab18d4c4b8e9403414c441847a62d697e1f4e0c68898e307ceb001": true,
	"88bc14017868ffac5c3036b570c80b3e2f13220b292f10353f1448a6cc49b78b": true,
	"89892e3c0b86ebb28b61bb198c593b73a91b27a6d8e024f20cabf983aaa97e1a": true,
	"8c26973807c965211c676b2f287279353209a709d0898b27923feb39238de945": true,
	"8c7870d7e1c55d0f06d77db49843efaf017371b7e28f4a71241ff1746f04a5fc": true,
	"8d417db2dd8bf5e3084d1e3f196d583849d81bdd4c00c70b9d39369e96b8c782": true,
	"8dd3d055a7f47ffbca287285dcfd201dde92823ccc484d30f11cc40f079b3a3e": true,
	"92c063d67e8f14db71d288e11266ffdaf69cbf115f7bab14f3bc2f68d25e0e30": true,
	"941ea0cbce0926d9f34ec667cee669b61fd39f4338fa0b91048d5749d8880d25": true,
	"988c59eb38eb1fab07f306c162c4e1853595ca26ef583d20b007f2e12be05bcc": true,
	"9e466ac372ed264083e2db3b3c6971bbbdfb79c67ead2d9e2bd6ec6bfaffdcfc": true,
	"a3103835ccac5930d0e044fd9830df91fc61ad50e6e10d30d8f5588b395ec6e7": true,
	"a4b821003296cd5083060adc331f74fba8372997b0dd75ac75b4cd3838c6c626": true,
	"aa4216dc80a7299ae6de08444bea2c588dc873631ebc563c0776d83a1b61e3ce": true,
	"aafccd83af8f4bb25b08547e14bf9e01f1593a237aa575ec1f5b5cf5ab87945f": true,
	"ad446951a423d46ebdb77b1a60f09980c5d4d80b27e18bbfd5da4346154c08a4": true,
	"b4665eba997ed5f662949c2e822b5e14adff401af0cc246945266f5fab80e35b": true,
	"b59070cae663230be039709cd61524861fcb74ce05a4386370482f6721ef2a74": true,
	"b5ee1c904ac2aae39c92601ab466269c0c1cd0b96badbd251b0ed88e6c4ded68": true,
	"b7408b4d2be0238ba37004dd34e276c6019bd2f24c9db7d4980f5f6c359a4bcc": true,
	"bb2b9b953e92b17de687fdc420edbf4efe5fb5170be34c02cc9d06f6b3a360a1": true,
	"bfd287ea8bbda66b68b51f70aa34a0bd7105fd8ef4212460a49b6dd63884eec2": true,
	"c073efb31422ff4db99dc24d7442438e942c73cd3c61b529a8ba32542e946b86": true,
	"c12ddf9f09fa23ed1a5fe511aef4623bb84f58f8d39e15f8ba2eaf024ced3a5b": true,
	"c2690b6bf35a9364e697735ec630bfd9d2cd59040e3908a66765168e3282b04e": true,
	"c48b0eb4de6e6796594118874aee8dad7837ea47a78cf81a6197a88fd951b179": true,
	"c619e4780ed8e0abeda24ad732567de4383a7ea941338f6d1a7d1b49ac792fe9": true,
	"c6fd1e9191185d5e2a09fc5d35674e0292b54ed426a98e3dc91b98443b186f0a": true,
	"c7bc3a6dc167d21d8664dd741e6cefd1f5493c00ffb6b9f7df06a02360d67718": true,
	"c844c56a21f48eac0437ecb3bbaa476ed9a3804e57cd3dddbe68bf5b36f7d613": true,
	"cdb39f94236daa3f6d57fe19ab930abc9f761cdb1e1916789a2c2fc5536d827b": true,
	"ceb5298daeb8f59279480b46002c89f7755cc3a169514b11b810befc195a00d8": true,
	"cf36ee3b065cb7405569f6c189b54c2f3d43042a995e3f5fcfccefe09ad73177": true,
	"d16f35a2fc245c9aea23050fb48f69240aeb2892466cf998c518139fbd0fec0e": true,
	"d45ce41f3b285436b8fd3e2b68e500f2835dd737035f2bcc489e9e48cf0cccd8": true,
	"d4ca6e7bca81ea0133c06ddf6ca73521d4f2757b1af8f4b6e8359294fd3c3470": true,
	"d6ec3e6cbf3dbd9ba738f88620d103de81ee0dd0ae8d8a599f0dcf9504a0b1fa": true,
	"d7b1b5dfa191b05a900ce37534f4d8eb247f500d50b9d1a4cc2777c28eec7333": true,
	"d7de9c05ce19a59f9d81cd9ff5e888cec4c3287d5470c510925a5eaa3b7cd54f": true,
	"d8aaa5319de5242ec1734b988bdf330fec7dbd38206caf76656ee4836a216876": true,
	"daa9724da8e1fe403aba0b412467db4e4b376c631eea8dcea4960929f4aeb9b8": true,
	"ddb1222b5107b05ff2c87e641d5f6c9d83be852fe28e28ef441092d70aeb02e7": true,
	"e0254c7fcdc265e2f0f8a428dabb75ab4ae039b4872d62f5ee57f0877d5a8a41": true,
	"e1ba9a97d89e75e299b49f95e907e42ef7232f6627da976f79c2bd32ab0039f9": true,
	"e1dda9bb414de47595a4f6d8bbed1c1f501e099633ddd833f5e15e634c6e4e1f": true,
	"e22969af379ab98524470b9c64a0b2b8cb907151fd7a7fc5cd1f2469bdb7154c": true,
	"e3212ac258d6d86f464c1150866a298c04a42783c1d17d7baa6a24ac0323f139": true,
	"e842a02a225a2377f842a6f0c245688cd8940bcc8cbf1e11a73c80f29889c3d2": true,
	"ea7068b8a34041200a6ccca7db67c29c8dddc312b47e24f852fcdc4e310c40aa": true,
	"eabc185c4e82d942b1a5978ba3c0181487d6b3b9974e5c49f72f6d0bd9637150": true,
	"ebed6475c2c2d80fcb199f3fe8dd6e6b3ce60b77eb15d4e42c3dec0c6f5b5105": true,
	"ec4ffa1ad0ac88081314b7aaffef6a91f97cd57291bb5ba686c5415f5eb4f6b6": true,
	"ece28e35289ce8621c72407949e5050052ad4607cc332eaf9b546029aacbfb7a": true,
	"ed5cb9c88d0b11bf5df5815e1222e13eeaad26683316307cbeab84b8673f7e65": true,
	"eef5de4a7457bcb5ef97d075cf337fc8209e4bc2473ecf681127ee7da0194cfa": true,
	"ef27c5e407461fbe59c7d92cb8219794694841c5e54db3c554e87971c693174a": true,
	"f1e62919c593884672332ca0d7d43d4fe6c242defcbc68ee24390b535d071b1f": true,
	"f4ed3c5ea849e062b9aa318d1fd218db85d0c611aff1db056db64304f370d710": true,
	"f641fa53945b53104aef0bb3e60b30e6634382e1981d2b10139f3562e2772fc3": true,
	"f8004e61e811655974f86b81c3787ff52148af9ad389f55a899cfc10b35c60d2": true,
	"f81088b361712f3aca87f8beacce5e4bc978ed4e9b9c085ba46fa2ea8941b2fd": true,
	"f8db20c2d076249967beea91ec27c954a5d405863e0ed1231c3b8c7f1fbfc7ff": true,
	"f8e4c9e8cd6ea6e4fbb8c68249a36394ffc3d00318aea8f0516084b9ed07db81": true,
	"f9990a4ea082fa7ff79287d3c190d850ee903a3b1ea91d5ebb5651fcdaf93d14": true,
	"fa1ea72910c99105c9f46ea1fd31dddada844d1f5daefdd1723abd73fdf934d1": true,
	"fa2e516c2bb85c813250bd791da34d1f110e9c4a8c78a8bd834197d083637bc8": true,
	"fff112fc55c9d5b2616a45455c326d290a17d38f06ec21116d624425da822728": true,
}
