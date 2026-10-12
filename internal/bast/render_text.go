package bast

// Nama orang, alamat, lokasi, dan nama penandatangan dikapitalkan di sini lalu
// dicetak huruf judul (textnorm.DisplayTitle); nomor seri tetap kapital. Data
// barang (merek, tipe, daya, BBM, nama komponen, satuan) dicetak apa adanya
// dari Template Paket, yang sudah ditulis sesuai EYD.

import "konkit/internal/textnorm"

func renderBusinessText(value string) string {
	return textnorm.BusinessUpper(value)
}

func uppercaseRecipientDocument(document RecipientDocument) RecipientDocument {
	snapshot := document.Snapshot
	snapshot.Recipient.FullName = renderBusinessText(snapshot.Recipient.FullName)
	snapshot.Recipient.SectorIdentifier = renderBusinessText(snapshot.Recipient.SectorIdentifier)
	snapshot.Recipient.Address = renderBusinessText(snapshot.Recipient.Address)
	snapshot.Recipient.Village = renderBusinessText(snapshot.Recipient.Village)
	snapshot.Recipient.District = renderBusinessText(snapshot.Recipient.District)
	snapshot.Recipient.Regency = renderBusinessText(snapshot.Recipient.Regency)
	snapshot.Equipment.MachineSerial = renderBusinessText(snapshot.Equipment.MachineSerial)
	snapshot.Equipment.HoseSerial = renderBusinessText(snapshot.Equipment.HoseSerial)
	snapshot.Equipment.ConverterSerial = renderBusinessText(snapshot.Equipment.ConverterSerial)
	snapshot.Signatures.ReceiverName = renderBusinessText(snapshot.Signatures.ReceiverName)
	snapshot.Signatures.ExecutorName = renderBusinessText(snapshot.Signatures.ExecutorName)
	snapshot.Signatures.SupervisorName = renderBusinessText(snapshot.Signatures.SupervisorName)
	document.Snapshot = snapshot
	return document
}

func uppercaseDP3Snapshot(snapshot DP3Snapshot) DP3Snapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.HandoverLocation = renderBusinessText(snapshot.HandoverLocation)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Recipients = append([]dp3RecipientSnapshot(nil), snapshot.Recipients...)
	for index := range snapshot.Recipients {
		recipient := &snapshot.Recipients[index]
		recipient.FullName = renderBusinessText(recipient.FullName)
		recipient.Address = renderBusinessText(recipient.Address)
		recipient.Village = renderBusinessText(recipient.Village)
		recipient.District = renderBusinessText(recipient.District)
		recipient.Regency = renderBusinessText(recipient.Regency)
	}
	return snapshot
}

func uppercaseDailyRecapSnapshot(snapshot DailyRecapSnapshot) DailyRecapSnapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.HandoverLocation = renderBusinessText(snapshot.HandoverLocation)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Signatories.PertaminaRepName = renderBusinessText(snapshot.Signatories.PertaminaRepName)
	snapshot.Recipients = append([]dailyRecapRecipientSnapshot(nil), snapshot.Recipients...)
	for index := range snapshot.Recipients {
		recipient := &snapshot.Recipients[index]
		recipient.FullName = renderBusinessText(recipient.FullName)
		recipient.FarmerCardNumber = renderBusinessText(recipient.FarmerCardNumber)
		recipient.MachineSerial = renderBusinessText(recipient.MachineSerial)
	}
	return snapshot
}

func uppercaseClosingKabupatenSnapshot(snapshot ClosingKabupatenSnapshot) ClosingKabupatenSnapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.ProvinceName = renderBusinessText(snapshot.ProvinceName)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Signatories.PertaminaRepName = renderBusinessText(snapshot.Signatories.PertaminaRepName)
	snapshot.Rows = append([]ClosingKabupatenRow(nil), snapshot.Rows...)
	for index := range snapshot.Rows {
		snapshot.Rows[index].Location = renderBusinessText(snapshot.Rows[index].Location)
	}
	return snapshot
}

func uppercaseClosingSnapshot(snapshot ClosingTitikSerahSnapshot) ClosingTitikSerahSnapshot {
	snapshot.RegencyName = renderBusinessText(snapshot.RegencyName)
	snapshot.ProvinceName = renderBusinessText(snapshot.ProvinceName)
	snapshot.HandoverLocation = renderBusinessText(snapshot.HandoverLocation)
	snapshot.ConsultantCompanyName = renderBusinessText(snapshot.ConsultantCompanyName)
	snapshot.Signatories.AgricultureOfficeName = renderBusinessText(snapshot.Signatories.AgricultureOfficeName)
	snapshot.Signatories.InstallerName = renderBusinessText(snapshot.Signatories.InstallerName)
	snapshot.Signatories.SupervisorName = renderBusinessText(snapshot.Signatories.SupervisorName)
	snapshot.Signatories.PertaminaRepName = renderBusinessText(snapshot.Signatories.PertaminaRepName)
	return snapshot
}
