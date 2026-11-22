package dna

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The API exposes many fields as bare integers (and a few as free-form
// strings that only accept specific words). The meanings below come from the
// API's server-side enum definitions; the OpenAPI document carries only the
// numbers. They power `dna enums`, table output (status 7 -> PendingDelete),
// input normalization (--dnssec-mode DisableDnssec -> 2) and MCP schemas.

// EnumValue is one allowed value of an enum.
type EnumValue struct {
	// Value is what goes on the wire: an int for integer enums, a string for
	// string enums.
	Value       any    `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// EnumInfo documents an enum: where it is used and what each value means.
type EnumInfo struct {
	Name        string      `json:"name"`
	Kind        string      `json:"kind"` // "integer" or "string"
	Description string      `json:"description"`
	Values      []EnumValue `json:"values"`
	// CaseSensitive is true when the API rejects values in the wrong case.
	CaseSensitive bool `json:"caseSensitive,omitempty"`
}

// Summary renders the values compactly, e.g. "0=Active, 1=WaitingForRegistration".
func (e *EnumInfo) Summary() string {
	parts := make([]string, 0, len(e.Values))
	for _, v := range e.Values {
		if e.Kind == "string" {
			parts = append(parts, v.Name)
		} else {
			parts = append(parts, fmt.Sprintf("%v=%s", v.Value, v.Name))
		}
	}
	return strings.Join(parts, ", ")
}

// NameOf returns the member name for a wire value, or "" if unknown.
func (e *EnumInfo) NameOf(v any) string {
	s := fmt.Sprint(v)
	for _, ev := range e.Values {
		if fmt.Sprint(ev.Value) == s {
			return ev.Name
		}
	}
	return ""
}

// Normalize maps user input (a name in any case, or a number for integer
// enums) to the exact wire value. ok is false when the input matches nothing.
func (e *EnumInfo) Normalize(in string) (value any, ok bool) {
	in = strings.TrimSpace(in)
	for _, ev := range e.Values {
		if strings.EqualFold(ev.Name, in) || fmt.Sprint(ev.Value) == in {
			return ev.Value, true
		}
	}
	if e.Kind == "integer" {
		if n, err := strconv.Atoi(in); err == nil {
			for _, ev := range e.Values {
				if ev.Value == n {
					return n, true
				}
			}
		}
	}
	return nil, false
}

var enumRegistry = map[string]*EnumInfo{}

func intEnum(name, desc string, values ...EnumValue) *EnumInfo {
	e := &EnumInfo{Name: name, Kind: "integer", Description: desc, Values: values}
	enumRegistry[name] = e
	return e
}

func strEnum(name, desc string, caseSensitive bool, values ...EnumValue) *EnumInfo {
	e := &EnumInfo{Name: name, Kind: "string", Description: desc, Values: values, CaseSensitive: caseSensitive}
	enumRegistry[name] = e
	return e
}

func ival(value any, name string, desc ...string) EnumValue {
	ev := EnumValue{Value: value, Name: name}
	if len(desc) > 0 {
		ev.Description = desc[0]
	}
	return ev
}

func sval(name string, desc ...string) EnumValue { return ival(name, name, desc...) }

// Enums returns every documented enum, sorted by name.
func Enums() []*EnumInfo {
	out := make([]*EnumInfo, 0, len(enumRegistry))
	for _, e := range enumRegistry {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LookupEnum finds an enum by name (case-insensitive).
func LookupEnum(name string) (*EnumInfo, bool) {
	for k, e := range enumRegistry {
		if strings.EqualFold(k, name) {
			return e, true
		}
	}
	return nil, false
}

// EnumFor returns the enum used by property prop of the API schema named
// schema (its short name, e.g. "DomainDto", "status").
func EnumFor(schema, prop string) (*EnumInfo, bool) {
	name, ok := enumFields[schema+"."+prop]
	if !ok {
		name, ok = stringEnumFields[schema+"."+prop]
	}
	if !ok {
		return nil, false
	}
	e, ok := enumRegistry[name]
	return e, ok
}

// enumFields maps "<Schema>.<property>" of integer enum properties to their
// enum. The OpenAPI document only carries the numbers; a test in
// internal/ops fails when the spec gains an integer enum missing here.
var enumFields = map[string]string{
	"AccountTransactionDto.chargeType":           "ChargeType",
	"AccountTransactionDto.productType":          "ProductType",
	"AccountTransactionDto.status":               "TransactionStatus",
	"AccountTransactionDto.type":                 "TransactionType",
	"DnsEditNameServerInput.dnssecMode":          "DNSSECMode",
	"DnssecInfoBaseDto.status":                   "DNSSECStatus",
	"DnssecOperationResultBaseDto.operationType": "DNSSECOperationType",
	"DomainDocumentInfoDto.operationType":        "DocumentOperationType",
	"DomainDocumentInfoDto.status":               "DocumentStatus",
	"DomainDocumentInfoDto.type":                 "DocumentType",
	"DomainDocumentUploadItemInput.documentType": "DocumentType",
	"DomainDocumentUploadedItemDto.type":         "DocumentType",
	"DomainDto.status":                           "DomainStatus",
	"DsRecordBaseDto.status":                     "DSRecordStatus",
	"OrderDto.status":                            "OrderStatus",
	"OrderProductDto.orderType":                  "OrderType",
	"OrderProductDto.productType":                "ProductType",
	"OrderProductDto.status":                     "OrderProductStatus",
	"ProductDto.productType":                     "ProductType",
	"ProductInfoDto.productType":                 "ProductType",
	"ResellerMeDto.accountCurrency":              "AccountCurrency",
	"ResellerMeDto.status":                       "ResellerStatus",
	"SubproductDto.productType":                  "ProductType",
}

// stringEnumFields lists string-typed properties that only accept the words
// of an enum. (Integer enums are listed in enumFields.)
var stringEnumFields = map[string]string{
	"ContactHandleDto.contactType":                 "ContactType",
	"DomainContactDetailCreateDto.contactType":     "ContactType",
	"ZoneCreateDto.type":                           "RecordType",
	"DomainForwardCreateInput.forwardType":         "ForwardType",
	"ConfigureCertificateInput.authenticationType": "AuthenticationType",
	"ConfigureCertificateInput.webServerType":      "WebServerType",
	"IpAddressInput.ipVersion":                     "IPVersion",
	"DsRecordInput.algorithm":                      "DNSSECAlgorithm",
	"DsRecordInput.digestType":                     "DSDigestType",
}

// ---------------------------------------------------------------------------
// Integer enums

var _ = intEnum("DomainStatus", "Lifecycle state of a domain (DomainDto.status, filter with `domain list --status-ids`).",
	ival(0, "Active"), ival(1, "WaitingForRegistration"), ival(2, "WaitingForDocument", "Registry documents must be uploaded"),
	ival(3, "WaitingForIncomingTransfer", "Internal transfer from another reseller"), ival(4, "TransferredOut"),
	ival(5, "TransferRejected"), ival(7, "PendingDelete"), ival(8, "Deleted"),
	ival(9, "ConfirmationEmailSend", "Transfer confirmation e-mail sent"), ival(11, "WaitingForOutgoingTransfer"),
	ival(12, "PendingHold"), ival(15, "ModificationPending"), ival(18, "InCase", "Subject to a legal process"), ival(19, "PendingRestore"))

var _ = intEnum("TransactionType", "Kind of balance transaction (AccountTransactionDto.type, filter `--transaction-types`).",
	ival(1, "Register"), ival(2, "Renew"), ival(3, "Transfer"), ival(4, "Restore"), ival(5, "Deposit"), ival(6, "Backorder"),
	ival(7, "Refund"), ival(8, "Deduction"), ival(9, "PreRegister"), ival(10, "Shopping"), ival(11, "ResellerPayment"), ival(12, "InternalTransfer"))

var _ = intEnum("TransactionStatus", "Outcome of a balance transaction (AccountTransactionDto.status).",
	ival(0, "Failed"), ival(1, "Success"), ival(2, "Canceled"), ival(3, "Processing"), ival(4, "Refunded"))

var _ = intEnum("ChargeType", "Direction of a balance transaction (AccountTransactionDto.chargeType, filter `--charge-type`).",
	ival(0, "Add"), ival(1, "Subtract"))

var _ = intEnum("AccountCurrency", "Currency a reseller account is kept in (ResellerMeDto.accountCurrency).",
	ival(1, "TRY"), ival(2, "USD"), ival(3, "EURO"))

var _ = intEnum("ResellerStatus", "State of a reseller account (ResellerMeDto.status).",
	ival(0, "Active"), ival(1, "WaitingForApproval", "E-mail or SMS verification pending"),
	ival(2, "Suspend", "Access suspended"), ival(3, "WaitingForCompletionWizard", "Sign-up wizard not finished"))

var _ = intEnum("ProductType", "Kind of product (productType fields; filter `product list --product-type`).",
	ival(1, "Domain"), ival(2, "SSL"), ival(3, "Deposit"), ival(4, "DomainTrustee"), ival(5, "Hosting"), ival(6, "OwnerChange"))

var _ = intEnum("OrderType", "Kind of order (OrderProductDto.orderType and the --order-type of `product info`).",
	ival(1, "Register"), ival(2, "Renew"), ival(3, "Transfer"), ival(4, "Restore"), ival(5, "SubresellerTransfer"), ival(6, "Backorder"),
	ival(7, "InternalTransfer"), ival(8, "Charge"), ival(9, "Trustee"), ival(10, "Leasing"), ival(11, "Create"), ival(12, "OwnerChange"))

var _ = intEnum("OrderStatus", "State of an order (OrderDto.status).",
	ival(1, "Active"), ival(2, "Pending"), ival(3, "Canceled"), ival(4, "InProgress"))

var _ = intEnum("OrderProductStatus", "State of an order line (OrderProductDto.status).",
	ival(1, "Ordered"), ival(2, "Canceled"), ival(3, "InProgress"))

var _ = intEnum("DNSSECMode", "What to do with DNSSEC when changing the nameservers of a signed domain (DnsEditNameServerInput.dnssecMode).",
	ival(0, "Block", "Reject the nameserver change while DS records exist (safe default)"),
	ival(1, "KeepDnssec", "Keep DS records; you confirm the new DNS provider serves a matching DNSKEY"),
	ival(2, "DisableDnssec", "Remove all DS records at the registry first, then change nameservers"))

var _ = intEnum("DNSSECStatus", "DNSSEC state of a domain (DnssecInfoBaseDto.status).",
	ival(0, "Unsigned", "No DS record at the registry"), ival(1, "PendingEnable", "DS add sent, awaiting confirmation"),
	ival(2, "Signed", "At least one active DS record"), ival(3, "PendingDisable", "DS removal sent, awaiting confirmation"),
	ival(4, "Error", "The last DNSSEC operation failed"))

var _ = intEnum("DNSSECOperationType", "Kind of DNSSEC operation (DnssecOperationResultBaseDto.operationType).",
	ival(0, "Add"), ival(1, "Remove"), ival(2, "RemoveAll"), ival(3, "Sync"))

var _ = intEnum("DSRecordStatus", "State of a DS record (DsRecordBaseDto.status). Several can be Active during a key rollover.",
	ival(0, "Pending"), ival(1, "Active"), ival(2, "Removing"), ival(3, "Removed"), ival(4, "Failed"))

var _ = intEnum("DocumentType", "Kind of registry document (documentType / type fields of domain documents).",
	ival(1, "IdentityCard"), ival(2, "Passport"), ival(3, "TaxCertificate"), ival(4, "ActivityCertificate"),
	ival(5, "SignatureCircular"), ival(6, "AuthorizationLetter"), ival(7, "Petition"), ival(8, "TradeRegistryGazette"),
	ival(9, "TrademarkCertificate"), ival(10, "EducationCertificate", ".edu.tr"), ival(11, "OrganizationStatute", ".org.tr"),
	ival(12, "GovernmentApproval", ".gov.tr, .bank, .mil.tr"), ival(13, "DomainOwnershipProof"), ival(14, "PowerOfAttorney"),
	ival(15, "BarAssociationCertificate", ".av.tr"), ival(16, "MedicalDiplomaOrLicense", ".dr.tr"),
	ival(17, "MunicipalOfficialLetter", ".bel.tr"), ival(18, "EducationInstitutionLicense", ".k12.tr, .edu.tr"),
	ival(19, "ChamberOfCommerceCertificate"), ival(20, "TrademarkTransferAgreement"), ival(21, "KepAuthorization", ".kep.tr"),
	ival(99, "Other"))

var _ = intEnum("DocumentStatus", "Review state of a registry document (DomainDocumentInfoDto.status).",
	ival(1, "Waiting", "Document expected"), ival(2, "Pending", "Sent, not yet reviewed"), ival(3, "Approved"),
	ival(4, "Rejected"), ival(5, "Expired"), ival(6, "Cancelled", "Application or owner change cancelled"))

var _ = intEnum("DocumentOperationType", "Operation a document was requested for (DomainDocumentInfoDto.operationType).",
	ival(1, "Create", "New registration"), ival(2, "OwnerChange", "Trade / registrant change"), ival(3, "Delete"))

// DNSSEC algorithm and digest numbers are IANA registry codes.
var _ = intEnum("DNSSECAlgorithm", "IANA DNSSEC algorithm number (DsRecordInput.algorithm).",
	ival(1, "RsaMd5"), ival(3, "Dsa"), ival(5, "RsaSha1"), ival(6, "DsaNsec3Sha1"), ival(7, "RsaSha1Nsec3Sha1"),
	ival(8, "RsaSha256"), ival(10, "RsaSha512"), ival(12, "EccGost"), ival(13, "EcdsaP256Sha256"),
	ival(14, "EcdsaP384Sha384"), ival(15, "Ed25519"), ival(16, "Ed448"))

var _ = intEnum("DSDigestType", "IANA DS digest type (DsRecordInput.digestType).",
	ival(1, "Sha1"), ival(2, "Sha256"), ival(3, "Gost"), ival(4, "Sha384"))

// ---------------------------------------------------------------------------
// String enums. The API accepts these words in string fields.

// Contact roles (contactType). The API silently treats unknown values as
// Registrant, so the CLI validates them strictly.
const (
	ContactTypeRegistrant = "Registrant"
	ContactTypeAdmin      = "Admin"
	ContactTypeTech       = "Tech"
	ContactTypeBilling    = "Billing"
)

var _ = strEnum("ContactType", "Contact role (contactType). Registering with contacts needs all four roles unless useTrusteeContact is true. Unknown values are silently treated as Registrant by the API.", false,
	sval("Registrant"), sval("Admin"), sval("Tech"), sval("Billing"))

// DNS record types accepted by zone records (ZoneCreateDto.type).
var _ = strEnum("RecordType", "DNS zone record type (ZoneCreateDto.type). The API is case-sensitive; the CLI normalizes the case for you. TXT content is quoted automatically; CAA content must have exactly 3 space-separated parts; CNAME needs a non-empty name.", true,
	sval("A"), sval("AAAA"), sval("ALIAS"), sval("NS"), sval("CNAME"), sval("MX"), sval("SOA"), sval("TXT"), sval("PTR"), sval("SRV"),
	sval("DNSKey"), sval("DS"), sval("NSEC"), sval("NSEC3"), sval("NSEC3Param"), sval("RRSIG"), sval("AFSDB"), sval("ATMA"), sval("CAA"),
	sval("CERT"), sval("DHCID"), sval("DName"), sval("HInfo"), sval("ISDN"), sval("LOC"), sval("MB"), sval("MG"), sval("MInfo"), sval("MR"),
	sval("NAPTR"), sval("NSAP"), sval("RP"), sval("RT"), sval("TLSA"), sval("X25"))

// URL forwarding modes (DomainForwardCreateInput.forwardType).
const (
	ForwardTypeStandard = "Standard"
	ForwardTypeFrame    = "Frame"
)

var _ = strEnum("ForwardType", "URL forwarding mode (forwardType).", false,
	sval("Standard", "HTTP 301 redirect"), sval("Frame", "Show the target inside a frame (masked URL)"))

// SSL domain-control validation methods (ConfigureCertificateInput.authenticationType).
var _ = strEnum("AuthenticationType", "SSL domain control validation method (authenticationType).", false,
	sval("Email", "Approval e-mail to approverMail"), sval("Cname", "DNS CNAME record"), sval("Http", "HTTP file on the site"), sval("File", "File-based validation"))

// Web server software of the certificate (ConfigureCertificateInput.webServerType).
var _ = strEnum("WebServerType", "Web server type of the SSL certificate (webServerType).", false,
	sval("Apache"), sval("IIS"), sval("AOL"), sval("C2Net"))

// IP versions of child nameserver (host) addresses (IpAddressInput.ipVersion).
const (
	IPVersion4 = "v4"
	IPVersion6 = "v6"
)

var _ = strEnum("IPVersion", "IP version of a child nameserver address (ipVersion). The CLI detects it from the address.", true,
	sval("v4"), sval("v6", "Rejected when the TLD does not support IPv6 glue"))
