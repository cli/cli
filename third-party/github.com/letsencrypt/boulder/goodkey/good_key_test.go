package goodkey

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

// testingPolicy is a simple policy which allows all of the key types, so that
// the unit tests can exercise checks against all key types.
var testingPolicy = &KeyPolicy{allowedKeys: AllowedKeys{
	RSA2048: true, RSA3072: true, RSA4096: true,
	ECDSAP256: true, ECDSAP384: true, ECDSAP521: true,
}}

func TestUnknownKeyType(t *testing.T) {
	notAKey := struct{}{}
	err := testingPolicy.GoodKey(context.Background(), notAKey)
	test.AssertError(t, err, "Should have rejected a key of unknown type")
	test.AssertEquals(t, err.Error(), "unsupported key type struct {}")
}

func TestNilKey(t *testing.T) {
	err := testingPolicy.GoodKey(context.Background(), nil)
	test.AssertError(t, err, "Should have rejected a nil key")
	test.AssertEquals(t, err.Error(), "unsupported key type <nil>")
}

func TestSmallModulus(t *testing.T) {
	pubKey := rsa.PublicKey{
		N: big.NewInt(0),
		E: 65537,
	}
	// 2040 bits
	_, ok := pubKey.N.SetString("104192126510885102608953552259747211060428328569316484779167706297543848858189721071301121307701498317286069484848193969810800653457088975832436062805901725915630417996487259956349018066196416400386483594314258078114607080545265502078791826837453107382149801328758721235866366842649389274931060463277516954884108984101391466769505088222180613883737986792254164577832157921425082478871935498631777878563742033332460445633026471887331001305450139473524438241478798689974351175769895824322173301257621327448162705637127373457350813027123239805772024171112299987923305882261194120410409098448380641378552305583392176287", 10)
	if !ok {
		t.Errorf("error parsing pubkey modulus")
	}
	err := testingPolicy.GoodKey(context.Background(), &pubKey)
	test.AssertError(t, err, "Should have rejected too-short key")
	test.AssertEquals(t, err.Error(), "key size not supported: 2040")
}

func TestLargeModulus(t *testing.T) {
	pubKey := rsa.PublicKey{
		N: big.NewInt(0),
		E: 65537,
	}
	// 4097 bits
	_, ok := pubKey.N.SetString("1528586537844618544364689295678280797814937047039447018548513699782432768815684971832418418955305671838918285565080181315448131784543332408348488544125812746629522583979538961638790013578302979210481729874191053412386396889481430969071543569003141391030053024684850548909056275565684242965892176703473950844930842702506635531145654194239072799616096020023445127233557468234181352398708456163013484600764686209741158795461806441111028922165846800488957692595308009319392149669715238691709012014980470238746838534949750493558807218940354555205690667168930634644030378921382266510932028134500172599110460167962515262077587741235811653717121760943005253103187409557573174347385738572144714188928416780963680160418832333908040737262282830643745963536624555340279793555475547508851494656512855403492456740439533790565640263514349940712999516725281940465613417922773583725174223806589481568984323871222072582132221706797917380250216291620957692131931099423995355390698925093903005385497308399692769135287821632877871068909305276870015125960884987746154344006895331078411141197233179446805991116541744285238281451294472577537413640009811940462311100056023815261650331552185459228689469446389165886801876700815724561451940764544990177661873073", 10)
	if !ok {
		t.Errorf("error parsing pubkey modulus")
	}
	err := testingPolicy.GoodKey(context.Background(), &pubKey)
	test.AssertError(t, err, "Should have rejected too-long key")
	test.AssertEquals(t, err.Error(), "key size not supported: 4097")
}

func TestModulusModulo8(t *testing.T) {
	bigOne := big.NewInt(1)
	key := rsa.PublicKey{
		N: bigOne.Lsh(bigOne, 2048),
		E: 5,
	}
	err := testingPolicy.GoodKey(context.Background(), &key)
	test.AssertError(t, err, "Should have rejected modulus with length not divisible by 8")
	test.AssertEquals(t, err.Error(), "key size not supported: 2049")
}

var mod2048 = big.NewInt(0).Sub(big.NewInt(0).Lsh(big.NewInt(1), 2048), big.NewInt(1))

func TestNonStandardExp(t *testing.T) {
	evenMod := big.NewInt(0).Add(big.NewInt(1).Lsh(big.NewInt(1), 2047), big.NewInt(2))
	key := rsa.PublicKey{
		N: evenMod,
		E: (1 << 16),
	}
	err := testingPolicy.GoodKey(context.Background(), &key)
	test.AssertError(t, err, "Should have rejected non-standard exponent")
	test.AssertEquals(t, err.Error(), "key exponent must be 65537")
}

func TestEvenModulus(t *testing.T) {
	evenMod := big.NewInt(0).Add(big.NewInt(1).Lsh(big.NewInt(1), 2047), big.NewInt(2))
	key := rsa.PublicKey{
		N: evenMod,
		E: (1 << 16) + 1,
	}
	err := testingPolicy.GoodKey(context.Background(), &key)
	test.AssertError(t, err, "Should have rejected even modulus")
	test.AssertEquals(t, err.Error(), "key divisible by small prime")
}

func TestModulusDivisibleBySmallPrime(t *testing.T) {
	key := rsa.PublicKey{
		N: mod2048,
		E: (1 << 16) + 1,
	}
	err := testingPolicy.GoodKey(context.Background(), &key)
	test.AssertError(t, err, "Should have rejected modulus divisible by 3")
	test.AssertEquals(t, err.Error(), "key divisible by small prime")
}

func TestROCA(t *testing.T) {
	n, ok := big.NewInt(1).SetString("19089470491547632015867380494603366846979936677899040455785311493700173635637619562546319438505971838982429681121352968394792665704951454132311441831732124044135181992768774222852895664400681270897445415599851900461316070972022018317962889565731866601557238345786316235456299813772607869009873279585912430769332375239444892105064608255089298943707214066350230292124208314161171265468111771687514518823144499250339825049199688099820304852696380797616737008621384107235756455735861506433065173933123259184114000282435500939123478591192413006994709825840573671701120771013072419520134975733578923370992644987545261926257", 10)
	if !ok {
		t.Fatal("failed to parse")
	}
	key := rsa.PublicKey{
		N: n,
		E: 65537,
	}
	err := testingPolicy.GoodKey(context.Background(), &key)
	test.AssertError(t, err, "Should have rejected ROCA-weak key")
	test.AssertEquals(t, err.Error(), "key generated by vulnerable Infineon-based hardware")
}

func TestGoodKey(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "Error generating key")
	test.AssertNotError(t, testingPolicy.GoodKey(context.Background(), &private.PublicKey), "Should have accepted good key")
}

func TestECDSABadCurve(t *testing.T) {
	for _, curve := range invalidCurves {
		private, err := ecdsa.GenerateKey(curve, rand.Reader)
		test.AssertNotError(t, err, "Error generating key")
		err = testingPolicy.GoodKey(context.Background(), &private.PublicKey)
		test.AssertError(t, err, "Should have rejected key with unsupported curve")
		test.AssertEquals(t, err.Error(), fmt.Sprintf("ECDSA curve %s not allowed", curve.Params().Name))
	}
}

var invalidCurves = []elliptic.Curve{
	elliptic.P224(),
}

var validCurves = []elliptic.Curve{
	elliptic.P256(),
	elliptic.P384(),
	elliptic.P521(),
}

func TestECDSAGoodKey(t *testing.T) {
	for _, curve := range validCurves {
		private, err := ecdsa.GenerateKey(curve, rand.Reader)
		test.AssertNotError(t, err, "Error generating key")
		test.AssertNotError(t, testingPolicy.GoodKey(context.Background(), &private.PublicKey), "Should have accepted good key")
	}
}

func TestECDSANotOnCurveX(t *testing.T) {
	for _, curve := range validCurves {
		// Change a public key so that it is no longer on the curve.
		private, err := ecdsa.GenerateKey(curve, rand.Reader)
		test.AssertNotError(t, err, "Error generating key")

		private.X.Add(private.X, big.NewInt(1))
		err = testingPolicy.GoodKey(context.Background(), &private.PublicKey)
		test.AssertError(t, err, "Should not have accepted key not on the curve")
		test.AssertEquals(t, err.Error(), "key point is not on the curve")
	}
}

func TestECDSANotOnCurveY(t *testing.T) {
	for _, curve := range validCurves {
		// Again with Y.
		private, err := ecdsa.GenerateKey(curve, rand.Reader)
		test.AssertNotError(t, err, "Error generating key")

		// Change the public key so that it is no longer on the curve.
		private.Y.Add(private.Y, big.NewInt(1))
		err = testingPolicy.GoodKey(context.Background(), &private.PublicKey)
		test.AssertError(t, err, "Should not have accepted key not on the curve")
		test.AssertEquals(t, err.Error(), "key point is not on the curve")
	}
}

func TestECDSANegative(t *testing.T) {
	for _, curve := range validCurves {
		// Check that negative X is not accepted.
		private, err := ecdsa.GenerateKey(curve, rand.Reader)
		test.AssertNotError(t, err, "Error generating key")

		private.X.Neg(private.X)
		err = testingPolicy.GoodKey(context.Background(), &private.PublicKey)
		test.AssertError(t, err, "Should not have accepted key with negative X")
		test.AssertEquals(t, err.Error(), "key x, y must not be negative")

		// Check that negative Y is not accepted.
		private.X.Neg(private.X)
		private.Y.Neg(private.Y)
		err = testingPolicy.GoodKey(context.Background(), &private.PublicKey)
		test.AssertError(t, err, "Should not have accepted key with negative Y")
		test.AssertEquals(t, err.Error(), "key x, y must not be negative")
	}
}

func TestECDSAXOutsideField(t *testing.T) {
	for _, curve := range validCurves {
		// Check that X outside [0, p-1] is not accepted.
		private, err := ecdsa.GenerateKey(curve, rand.Reader)
		test.AssertNotError(t, err, "Error generating key")

		private.X.Mul(private.X, private.Curve.Params().P)
		err = testingPolicy.GoodKey(context.Background(), &private.PublicKey)
		test.AssertError(t, err, "Should not have accepted key with a X > p-1")
		test.AssertEquals(t, err.Error(), "key x, y must not exceed P-1")
	}
}

func TestECDSAYOutsideField(t *testing.T) {
	for _, curve := range validCurves {
		// Check that Y outside [0, p-1] is not accepted.
		private, err := ecdsa.GenerateKey(curve, rand.Reader)
		test.AssertNotError(t, err, "Error generating key")

		private.X.Mul(private.Y, private.Curve.Params().P)
		err = testingPolicy.GoodKey(context.Background(), &private.PublicKey)
		test.AssertError(t, err, "Should not have accepted key with a Y > p-1")
		test.AssertEquals(t, err.Error(), "key x, y must not exceed P-1")
	}
}

func TestECDSAIdentity(t *testing.T) {
	for _, curve := range validCurves {
		// The point at infinity is 0,0, it should not be accepted.
		public := ecdsa.PublicKey{
			Curve: curve,
			X:     big.NewInt(0),
			Y:     big.NewInt(0),
		}

		err := testingPolicy.GoodKey(context.Background(), &public)
		test.AssertError(t, err, "Should not have accepted key with point at infinity")
		test.AssertEquals(t, err.Error(), "key x, y must not be the point at infinity")
	}
}

func TestNonRefKey(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "Error generating key")
	test.AssertError(t, testingPolicy.GoodKey(context.Background(), private.PublicKey), "Accepted non-reference key")
}

func TestDBBlocklistAccept(t *testing.T) {
	for _, testCheck := range []BlockedKeyCheckFunc{
		nil,
		func(context.Context, []byte) (bool, error) {
			return false, nil
		},
	} {
		policy, err := NewPolicy(nil, testCheck)
		test.AssertNotError(t, err, "NewKeyPolicy failed")

		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		test.AssertNotError(t, err, "ecdsa.GenerateKey failed")
		err = policy.GoodKey(context.Background(), k.Public())
		test.AssertNotError(t, err, "GoodKey failed with a non-blocked key")
	}
}

func TestDBBlocklistReject(t *testing.T) {
	testCheck := func(context.Context, []byte) (bool, error) {
		return true, nil
	}

	policy, err := NewPolicy(nil, testCheck)
	test.AssertNotError(t, err, "NewKeyPolicy failed")

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "ecdsa.GenerateKey failed")
	err = policy.GoodKey(context.Background(), k.Public())
	test.AssertError(t, err, "GoodKey didn't fail with a blocked key")
	test.AssertErrorIs(t, err, ErrBadKey)
	test.AssertEquals(t, err.Error(), "public key is forbidden")
}

func TestDefaultAllowedKeys(t *testing.T) {
	policy, err := NewPolicy(nil, nil)
	test.AssertNotError(t, err, "NewPolicy with nil config failed")
	test.Assert(t, policy.allowedKeys.RSA2048, "RSA 2048 should be allowed")
	test.Assert(t, policy.allowedKeys.RSA3072, "RSA 3072 should be allowed")
	test.Assert(t, policy.allowedKeys.RSA4096, "RSA 4096 should be allowed")
	test.Assert(t, policy.allowedKeys.ECDSAP256, "NIST P256 should be allowed")
	test.Assert(t, policy.allowedKeys.ECDSAP384, "NIST P384 should be allowed")
	test.Assert(t, !policy.allowedKeys.ECDSAP521, "NIST P521 should not be allowed")

	policy, err = NewPolicy(&Config{}, nil)
	test.AssertNotError(t, err, "NewPolicy with nil config.AllowedKeys failed")
	test.Assert(t, policy.allowedKeys.RSA2048, "RSA 2048 should be allowed")
	test.Assert(t, policy.allowedKeys.RSA3072, "RSA 3072 should be allowed")
	test.Assert(t, policy.allowedKeys.RSA4096, "RSA 4096 should be allowed")
	test.Assert(t, policy.allowedKeys.ECDSAP256, "NIST P256 should be allowed")
	test.Assert(t, policy.allowedKeys.ECDSAP384, "NIST P384 should be allowed")
	test.Assert(t, !policy.allowedKeys.ECDSAP521, "NIST P521 should not be allowed")
}

func TestRSAStrangeSize(t *testing.T) {
	k := &rsa.PublicKey{N: big.NewInt(10)}
	err := testingPolicy.GoodKey(context.Background(), k)
	test.AssertError(t, err, "expected GoodKey to fail")
	test.AssertEquals(t, err.Error(), "key size not supported: 4")
}

func TestCheckPrimeFactorsTooClose(t *testing.T) {
	type testCase struct {
		name         string
		p            string
		q            string
		expectRounds int
	}

	testCases := []testCase{
		{
			// The factors 59 and 101 multiply to 5959. The values a and b calculated
			// by Fermat's method will be 80 and 21. The ceil of the square root of
			// 5959 is 78. Therefore it takes 3 rounds of Fermat's method to find the
			// factors.
			name:         "tiny",
			p:            "101",
			q:            "59",
			expectRounds: 3,
		},
		{
			// These factors differ only in their second-to-last digit. They're so close
			// that a single iteration of Fermat's method is sufficient to find them.
			name:         "very close",
			p:            "12451309173743450529024753538187635497858772172998414407116324997634262083672423797183640278969532658774374576700091736519352600717664126766443002156788367",
			q:            "12451309173743450529024753538187635497858772172998414407116324997634262083672423797183640278969532658774374576700091736519352600717664126766443002156788337",
			expectRounds: 1,
		},
		{
			// These factors differ by slightly more than 2^256, which takes fourteen
			// rounds to factor.
			name:         "still too close",
			p:            "11779932606551869095289494662458707049283241949932278009554252037480401854504909149712949171865707598142483830639739537075502512627849249573564209082969463",
			q:            "11779932606551869095289494662458707049283241949932278009554252037480401854503793357623711855670284027157475142731886267090836872063809791989556295953329083",
			expectRounds: 14,
		},
		{
			// These factors come from a real canon printer in the wild with a broken
			// key generation mechanism.
			name:         "canon printer (2048 bit, 1 round)",
			p:            "155536235030272749691472293262418471207550926406427515178205576891522284497518443889075039382254334975506248481615035474816604875321501901699955105345417152355947783063521554077194367454070647740704883461064399268622437721385112646454393005862535727615809073410746393326688230040267160616554768771412289114449",
			q:            "155536235030272749691472293262418471207550926406427515178205576891522284497518443889075039382254334975506248481615035474816604875321501901699955105345417152355947783063521554077194367454070647740704883461064399268622437721385112646454393005862535727615809073410746393326688230040267160616554768771412289114113",
			expectRounds: 1,
		},
		{
			// These factors come from a real innsbruck printer in the wild with a
			// broken key generation mechanism.
			name:         "innsbruck printer (4096 bit, 1 round)",
			p:            "25868808535211632564072019392873831934145242707953960515208595626279836366691068618582894100813803673421320899654654938470888358089618966238341690624345530870988951109006149164192566967552401505863871260691612081236189439839963332690997129144163260418447718577834226720411404568398865166471102885763673744513186211985402019037772108416694793355840983833695882936201196462579254234744648546792097397517107797153785052856301942321429858537224127598198913168345965493941246097657533085617002572245972336841716321849601971924830462771411171570422802773095537171762650402420866468579928479284978914972383512240254605625661",
			q:            "25868808535211632564072019392873831934145242707953960515208595626279836366691068618582894100813803673421320899654654938470888358089618966238341690624345530870988951109006149164192566967552401505863871260691612081236189439839963332690997129144163260418447718577834226720411404568398865166471102885763673744513186211985402019037772108416694793355840983833695882936201196462579254234744648546792097397517107797153785052856301942321429858537224127598198913168345965493941246097657533085617002572245972336841716321849601971924830462771411171570422802773095537171762650402420866468579928479284978914972383512240254605624819",
			expectRounds: 1,
		},
		{
			// FIPS requires that |p-q| > 2^(nlen/2 - 100). For example, a 2048-bit
			// RSA key must have prime factors with a difference of at least 2^924.
			// These two factors have a difference of exactly 2^924 + 4, just *barely*
			// FIPS-compliant. Their first different digit is in column 52 of this
			// file, which makes them vastly further apart than the cases above. Their
			// product cannot be factored even with 100,000,000 rounds of Fermat's
			// Algorithm.
			name:         "barely FIPS compliant (2048 bit)",
			p:            "151546560166767007654995655231369126386504564489055366370313539237722892921762327477057109592614214965864835328962951695621854530739049166771701397343693962526456985866167580660948398404000483264137738772983130282095332559392185543017295488346592188097443414824871619976114874896240350402349774470198190454623",
			q:            "151546560166767007654995655231510939369872272987323309037144546294925352276321214430320942815891873491060949332482502812040326472743233767963240491605860423063942576391584034077877871768428333113881339606298282107984376151546711223157061364850161576363709081794948857957944390170575452970542651659150041855843",
			expectRounds: -1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := new(big.Int).SetString(tc.p, 10)
			if !ok {
				t.Fatalf("failed to load prime factor p (%s)", tc.p)
			}

			q, ok := new(big.Int).SetString(tc.q, 10)
			if !ok {
				t.Fatalf("failed to load prime factor q (%s)", tc.q)
			}

			n := new(big.Int).Mul(p, q)
			err := checkPrimeFactorsTooClose(n, 100)

			if tc.expectRounds > 0 {
				test.AssertError(t, err, "failed to factor n")
				test.AssertContains(t, err.Error(), fmt.Sprintf("p: %s", tc.p))
				test.AssertContains(t, err.Error(), fmt.Sprintf("q: %s", tc.q))
				test.AssertContains(t, err.Error(), fmt.Sprintf("in %d rounds", tc.expectRounds))
			} else {
				test.AssertNil(t, err, "factored the unfactorable")
			}
		})
	}
}

func benchFermat(rounds int, b *testing.B) {
	n := big.NewInt(0)
	n.SetString("801622717394169050106926578578301725055526605503706912100006286161529273473377413824975745384114446662904851914935980611269769546695796451504160869649117000521094368058953989236438103975426680952076533198797388295193391779933559668812684470909409457778161223896975426492372231040386646816154793996920467596916193680611886097694746368434138296683172992347929528214464827172059378866098534956467670429228681248968588692628197119606249988365750115578731538804653322115223303388019261933988266126675740797091559541980722545880793708750882230374320698192373040882555154628949384420712168289605526223733016176898368282023301917856921049583659644200174763940543991507836551835324807116188739389620816364505209568211448815747330488813651206715564392791134964121857454359816296832013457790067067190116393364546525054134704119475840526673114964766611499226043189928040037210929720682839683846078550615582181112536768195193557758454282232948765374797970874053642822355832904812487562117265271449547063765654262549173209805579494164339236981348054782533307762260970390747872669357067489756517340817289701322583209366268084923373164395703994945233187987667632964509271169622904359262117908604555420100186491963838567445541249128944592555657626247", 10)
	for range b.N {
		if checkPrimeFactorsTooClose(n, rounds) != nil {
			b.Fatal("factored the unfactorable!")
		}
	}
}

func BenchmarkFermat1(b *testing.B)     { benchFermat(1, b) }
func BenchmarkFermat10(b *testing.B)    { benchFermat(10, b) }
func BenchmarkFermat100(b *testing.B)   { benchFermat(100, b) }
func BenchmarkFermat1000(b *testing.B)  { benchFermat(1000, b) }
func BenchmarkFermat10000(b *testing.B) { benchFermat(10000, b) }
